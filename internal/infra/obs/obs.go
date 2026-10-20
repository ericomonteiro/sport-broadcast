// Package obs talks to OBS Studio through its WebSocket server (obs-websocket 5.x).
//
// Run keeps a connection open and reconnects when it drops, so the application
// starts even when OBS is closed. When OBS saves a replay, the replay is played
// in the replay scene and the scene that was on air comes back afterwards. A
// shorter replay (10s or 15s) starts part-way through the saved clip, so that
// only its last seconds are shown.
package obs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	goobs "github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/events"
	"github.com/andreykaipov/goobs/api/events/subscriptions"
	"github.com/andreykaipov/goobs/api/requests/inputs"
	"github.com/andreykaipov/goobs/api/requests/mediainputs"
	"github.com/andreykaipov/goobs/api/requests/outputs"
	"github.com/andreykaipov/goobs/api/requests/sceneitems"
	"github.com/andreykaipov/goobs/api/requests/scenes"
)

const (
	retryDelay      = 5 * time.Second
	pollInterval    = 250 * time.Millisecond
	startTimeout    = 10 * time.Second // how long to wait for the clip to start playing
	playbackTimeout = 2 * time.Minute  // upper bound for one clip, in case OBS never reports its end
	queueSize       = 8
	restartAction   = "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART"
	// fullReplaySeconds is the length of the whole replay buffer. Clips saved
	// without a requested length (for example by the OBS hotkey) play in full.
	fullReplaySeconds = 20
)

var errNotConnected = errors.New("not connected to OBS")

// Client sends commands to OBS and plays the saved replays. It is safe for concurrent use.
type Client struct {
	addr         string
	password     string
	replayScene  string
	replaySource string
	replayIntro  string
	replayLive   string

	mu   sync.Mutex
	conn *goobs.Client

	saved chan clip // replay files waiting to be played

	// pending holds the lengths asked for by SaveReplay, in request order. OBS
	// reports each saved file in the same order, so the next event takes the first.
	pending []int
}

// clip is a saved replay file and the number of seconds to show from its end.
type clip struct {
	path    string
	seconds int
}

// New creates a Client. replayScene is the OBS scene where replays play,
// replaySource is the media source with the clip, replayIntro plays before each
// clip, and replayLive plays after it to return to the live picture. An empty
// intro or live name skips that step. It does not connect yet; call Run.
func New(addr, password, replayScene, replaySource, replayIntro, replayLive string) *Client {
	return &Client{
		addr:         addr,
		password:     password,
		replayScene:  replayScene,
		replaySource: replaySource,
		replayIntro:  replayIntro,
		replayLive:   replayLive,
		saved:        make(chan clip, queueSize),
	}
}

// Run connects to OBS and reconnects after a failure, until ctx is done.
// Replays are played one at a time, in the order OBS saved them.
func (c *Client) Run(ctx context.Context) {
	go c.playLoop(ctx)

	failing := false
	for {
		conn, err := c.dial()
		if err != nil {
			if !failing {
				log.Printf("OBS indisponível em %s, tentando de novo a cada %s: %v", c.addr, retryDelay, err)
				failing = true
			}
		} else {
			if failing {
				log.Printf("conectado ao OBS em %s", c.addr)
			}
			failing = false
			c.setConn(conn)
			conn.Listen(c.onEvent) // returns when the connection closes
			c.setConn(nil)
			log.Printf("conexão com o OBS encerrada")
		}

		select {
		case <-ctx.Done():
			c.Close()
			return
		case <-time.After(retryDelay):
		}
	}
}

// SaveReplay tells OBS to save the replay buffer to a file. seconds is how much of
// the end of the saved clip to play (at most the buffer length set in OBS). The
// saved file is then played by Run through the replay event.
func (c *Client) SaveReplay(seconds int) error {
	conn := c.current()
	if conn == nil {
		return errNotConnected
	}

	// Queue the length before asking OBS, because the saved event can arrive
	// before the response does.
	c.mu.Lock()
	c.pending = append(c.pending, seconds)
	c.mu.Unlock()

	if _, err := conn.Outputs.SaveReplayBuffer(&outputs.SaveReplayBufferParams{}); err != nil {
		c.mu.Lock()
		c.pending = c.pending[:len(c.pending)-1]
		c.mu.Unlock()
		return fmt.Errorf("saving replay buffer: %w", err)
	}
	return nil
}

// Close closes the connection, if one is open.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		if err := c.conn.Disconnect(); err != nil {
			log.Printf("closing OBS connection: %v", err)
		}
		c.conn = nil
	}
}

func (c *Client) dial() (*goobs.Client, error) {
	return goobs.New(
		c.addr,
		goobs.WithPassword(c.password),
		goobs.WithEventSubscriptions(subscriptions.Outputs),
	)
}

func (c *Client) current() *goobs.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn
}

func (c *Client) setConn(conn *goobs.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
}

// onEvent runs in the goroutine that reads OBS events. It must not block.
func (c *Client) onEvent(event any) {
	switch e := event.(type) {
	case *events.ReplayBufferSaved:
		next := clip{path: e.SavedReplayPath, seconds: fullReplaySeconds}
		c.mu.Lock()
		if len(c.pending) > 0 {
			next.seconds = c.pending[0]
			c.pending = c.pending[1:]
		}
		c.mu.Unlock()

		select {
		case c.saved <- next:
		default:
			log.Printf("replay queue is full, not playing %s", e.SavedReplayPath)
		}
	}
}

// playLoop plays the queued replays one at a time, until ctx is done.
func (c *Client) playLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case next := <-c.saved:
			if err := c.play(next); err != nil {
				log.Printf("playing replay %s: %v", next.path, err)
			}
		}
	}
}

// play shows the replay scene, plays the intro (if configured) and then the clip,
// and goes back to the scene that was on air before.
func (c *Client) play(next clip) error {
	conn := c.current()
	if conn == nil {
		return errNotConnected
	}

	current, err := conn.Scenes.GetCurrentProgramScene(&scenes.GetCurrentProgramSceneParams{})
	if err != nil {
		return fmt.Errorf("reading current scene: %w", err)
	}
	previous := current.CurrentProgramSceneName

	// Load the clip before the intro starts, so it is ready when the intro ends.
	if _, err := conn.Inputs.SetInputSettings(&inputs.SetInputSettingsParams{
		InputName: &c.replaySource,
		InputSettings: map[string]any{
			"is_local_file": true,
			"local_file":    next.path,
			"looping":       false, // with looping on, the clip never reports its end
		},
	}); err != nil {
		return fmt.Errorf("setting replay file: %w", err)
	}

	// Sources with "restart when active" start as soon as the scene goes on air, so
	// the clip and the live source stay hidden until their turn.
	c.hide(conn, c.replaySource, c.replayLive)

	if _, err := conn.Scenes.SetCurrentProgramScene(scenes.NewSetCurrentProgramSceneParams().WithSceneName(c.replayScene)); err != nil {
		return fmt.Errorf("switching to replay scene: %w", err)
	}

	if c.replayIntro != "" {
		c.playInReplayScene(conn, c.replayIntro)
	}

	if err := c.setVisible(conn, c.replaySource, true); err != nil {
		c.backTo(conn, previous)
		return fmt.Errorf("showing replay clip: %w", err)
	}
	if err := c.startSource(conn, c.replaySource); err != nil {
		c.backTo(conn, previous)
		return fmt.Errorf("starting replay: %w", err)
	}
	if next.seconds < fullReplaySeconds {
		if err := c.seekToLast(conn, next.seconds); err != nil {
			log.Printf("starting %d s replay from the beginning: %v", next.seconds, err)
		}
	}
	c.waitForEnd(conn, c.replaySource)
	if err := c.setVisible(conn, c.replaySource, false); err != nil {
		log.Printf("hiding replay clip: %v", err)
	}

	if c.replayLive != "" {
		c.playInReplayScene(conn, c.replayLive)
	}
	c.backTo(conn, previous)
	log.Printf("back to scene %q", previous)
	return nil
}

// playInReplayScene shows a media source of the replay scene, waits until it has
// played completely, and then hides it so its last frame does not stay on screen.
// A failure only skips this source; the rest of the replay still runs.
func (c *Client) playInReplayScene(conn *goobs.Client, name string) {
	if err := c.setVisible(conn, name, true); err != nil {
		log.Printf("%s not shown: %v", name, err)
		return
	}
	if err := c.startSource(conn, name); err != nil {
		log.Printf("%s not played: %v", name, err)
	} else {
		c.waitForEnd(conn, name)
	}

	if err := c.setVisible(conn, name, false); err != nil {
		log.Printf("hiding %s: %v", name, err)
	}
}

// hide hides the given sources inside the replay scene. Failures are logged,
// because a missing source should not stop the replay.
func (c *Client) hide(conn *goobs.Client, names ...string) {
	for _, name := range names {
		if name == "" {
			continue
		}
		if err := c.setVisible(conn, name, false); err != nil {
			log.Printf("hiding %s before the replay: %v", name, err)
		}
	}
}

// setVisible shows or hides a source inside the replay scene.
func (c *Client) setVisible(conn *goobs.Client, source string, visible bool) error {
	item, err := conn.SceneItems.GetSceneItemId(sceneitems.NewGetSceneItemIdParams().WithSceneName(c.replayScene).WithSourceName(source))
	if err != nil {
		return fmt.Errorf("finding %q in scene %q: %w", source, c.replayScene, err)
	}
	if _, err := conn.SceneItems.SetSceneItemEnabled(sceneitems.NewSetSceneItemEnabledParams().WithSceneName(c.replayScene).WithSceneItemId(item.SceneItemId).WithSceneItemEnabled(visible)); err != nil {
		return fmt.Errorf("setting visibility of %q: %w", source, err)
	}
	return nil
}

// startSource turns off looping and restarts a media source from the beginning.
func (c *Client) startSource(conn *goobs.Client, name string) error {
	if _, err := conn.Inputs.SetInputSettings(&inputs.SetInputSettingsParams{
		InputName:     &name,
		InputSettings: map[string]any{"looping": false},
	}); err != nil {
		return fmt.Errorf("setting %q: %w", name, err)
	}
	action := restartAction
	if _, err := conn.MediaInputs.TriggerMediaInputAction(&mediainputs.TriggerMediaInputActionParams{
		InputName:   &name,
		MediaAction: &action,
	}); err != nil {
		return fmt.Errorf("restarting %q: %w", name, err)
	}
	return nil
}

// seekToLast moves the clip so that only its last `seconds` are played. OBS opens
// the file asynchronously, so this waits for PLAYING and then seeks once. It does
// not retry: repeated seeks made the clip jump back over and over. The position
// is read back and logged, so a seek that did not stick is visible in the log.
func (c *Client) seekToLast(conn *goobs.Client, seconds int) error {
	deadline := time.Now().Add(startTimeout)
	var last string

	for time.Now().Before(deadline) {
		status, err := conn.MediaInputs.GetMediaInputStatus(mediainputs.NewGetMediaInputStatusParams().WithInputName(c.replaySource))
		if err != nil {
			return fmt.Errorf("reading replay status: %w", err)
		}
		last = status.MediaState
		if status.MediaState == "OBS_MEDIA_STATE_PLAYING" && status.MediaDuration > 0 {
			offset := seekOffset(status.MediaDuration, seconds)
			if offset == 0 {
				return nil
			}
			if _, err := conn.MediaInputs.SetMediaInputCursor(mediainputs.NewSetMediaInputCursorParams().WithInputName(c.replaySource).WithMediaCursor(offset)); err != nil {
				return fmt.Errorf("seeking replay: %w", err)
			}
			time.Sleep(2 * pollInterval)
			after, err := conn.MediaInputs.GetMediaInputStatus(mediainputs.NewGetMediaInputStatusParams().WithInputName(c.replaySource))
			if err != nil {
				return fmt.Errorf("reading replay position: %w", err)
			}
			log.Printf("seek to %.1fs of %.1fs: cursor now %.1fs (state %s)", offset/1000, status.MediaDuration/1000, after.MediaCursor/1000, after.MediaState)
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("replay did not reach PLAYING within %s (last state %q)", startTimeout, last)
}

// seekOffset returns the position, in milliseconds, where a clip of durationMs
// starts so that only its last `seconds` remain. It never goes before the start.
func seekOffset(durationMs float64, seconds int) float64 {
	offset := durationMs - float64(seconds)*1000
	if offset < 0 {
		return 0
	}
	return offset
}

// waitForEnd polls a media source until it has played and stopped. It asks OBS
// directly instead of waiting for the MediaInputPlaybackEnded event, so it does
// not depend on that event arriving. Each state change is logged.
func (c *Client) waitForEnd(conn *goobs.Client, name string) {
	started := time.Now()
	deadline := started.Add(playbackTimeout)
	playing := false
	last := ""

	for time.Now().Before(deadline) {
		status, err := conn.MediaInputs.GetMediaInputStatus(mediainputs.NewGetMediaInputStatusParams().WithInputName(name))
		if err != nil {
			log.Printf("reading status of %q: %v", name, err)
			return
		}
		state := status.MediaState
		if state != last {
			log.Printf("%s state: %s (cursor %.1fs of %.1fs)", name, state, status.MediaCursor/1000, status.MediaDuration/1000)
			last = state
		}

		switch state {
		case "OBS_MEDIA_STATE_OPENING", "OBS_MEDIA_STATE_BUFFERING", "OBS_MEDIA_STATE_PLAYING", "OBS_MEDIA_STATE_PAUSED":
			playing = true
		case "OBS_MEDIA_STATE_ENDED", "OBS_MEDIA_STATE_STOPPED", "OBS_MEDIA_STATE_ERROR", "OBS_MEDIA_STATE_NONE":
			if playing {
				return
			}
		}
		if !playing && time.Since(started) > startTimeout {
			log.Printf("%s did not start within %s (state %q)", name, startTimeout, state)
			return
		}
		time.Sleep(pollInterval)
	}
	log.Printf("%s did not end within %s, going on anyway", name, playbackTimeout)
}

func (c *Client) backTo(conn *goobs.Client, scene string) {
	if _, err := conn.Scenes.SetCurrentProgramScene(scenes.NewSetCurrentProgramSceneParams().WithSceneName(scene)); err != nil {
		log.Printf("returning to scene %q: %v", scene, err)
	}
}
