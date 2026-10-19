// Package obs talks to OBS Studio through its WebSocket server (obs-websocket 5.x).
//
// Run keeps a connection open and reconnects when it drops, so the application
// starts even when OBS is closed. When OBS saves a replay, the replay is played
// in the replay scene and the scene that was on air comes back afterwards.
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
)

var errNotConnected = errors.New("not connected to OBS")

// Client sends commands to OBS and plays the saved replays. It is safe for concurrent use.
type Client struct {
	addr         string
	password     string
	replayScene  string
	replaySource string
	replayIntro  string

	mu   sync.Mutex
	conn *goobs.Client

	saved chan string // replay files waiting to be played
}

// New creates a Client. replayScene is the OBS scene where replays play,
// replaySource is the media source with the clip, and replayIntro is the media
// source that plays before each clip (empty to skip it). It does not connect yet; call Run.
func New(addr, password, replayScene, replaySource, replayIntro string) *Client {
	return &Client{
		addr:         addr,
		password:     password,
		replayScene:  replayScene,
		replaySource: replaySource,
		replayIntro:  replayIntro,
		saved:        make(chan string, queueSize),
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

// SaveReplay tells OBS to save the replay buffer to a file. The length of the
// clip is the buffer length set in OBS (Settings > Output > Replay Buffer).
// The saved file is then played by Run through the replay event.
func (c *Client) SaveReplay() error {
	conn := c.current()
	if conn == nil {
		return errNotConnected
	}
	if _, err := conn.Outputs.SaveReplayBuffer(&outputs.SaveReplayBufferParams{}); err != nil {
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
		select {
		case c.saved <- e.SavedReplayPath:
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
		case path := <-c.saved:
			if err := c.play(path); err != nil {
				log.Printf("playing replay %s: %v", path, err)
			}
		}
	}
}

// play shows the replay scene, plays the intro (if configured) and then the clip,
// and goes back to the scene that was on air before.
func (c *Client) play(path string) error {
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
			"local_file":    path,
			"looping":       false, // with looping on, the clip never reports its end
		},
	}); err != nil {
		return fmt.Errorf("setting replay file: %w", err)
	}

	if _, err := conn.Scenes.SetCurrentProgramScene(scenes.NewSetCurrentProgramSceneParams().WithSceneName(c.replayScene)); err != nil {
		return fmt.Errorf("switching to replay scene: %w", err)
	}

	if c.replayIntro != "" {
		c.playIntro(conn)
	}

	if err := c.startSource(conn, c.replaySource); err != nil {
		c.backTo(conn, previous)
		return fmt.Errorf("starting replay: %w", err)
	}
	c.waitForEnd(conn, c.replaySource)
	c.backTo(conn, previous)
	log.Printf("back to scene %q", previous)
	return nil
}

// playIntro shows the intro and waits until it has played completely, then hides
// it so its last frame does not cover the clip. A failure only skips the intro:
// the clip still plays.
func (c *Client) playIntro(conn *goobs.Client) {
	if err := c.setVisible(conn, c.replayIntro, true); err != nil {
		log.Printf("replay intro %q not shown: %v", c.replayIntro, err)
		return
	}
	if err := c.startSource(conn, c.replayIntro); err != nil {
		log.Printf("replay intro %q not played: %v", c.replayIntro, err)
	} else {
		c.waitForEnd(conn, c.replayIntro)
	}

	if err := c.setVisible(conn, c.replayIntro, false); err != nil {
		log.Printf("hiding replay intro %q: %v", c.replayIntro, err)
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
