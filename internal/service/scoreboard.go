package service

import (
	"context"
	"errors"
	"sync"

	"broadcast/internal/scoreboard"
)

// subscriberBuffer is how many updates a slow client can fall behind before the oldest are dropped.
const subscriberBuffer = 8

// ScoreboardService holds the live scoreboards. Each game is loaded from the
// database on first use and kept in memory. Every change is checked against
// the game's status, saved, and only then sent to subscribers.
type ScoreboardService struct {
	mu          sync.Mutex
	games       GameStore
	boards      ScoreboardStore
	active      map[int64]*scoreboard.Scoreboard
	subscribers map[int64]map[chan scoreboard.View]struct{}
}

// NewScoreboardService creates a ScoreboardService.
func NewScoreboardService(games GameStore, boards ScoreboardStore) *ScoreboardService {
	return &ScoreboardService{
		games:       games,
		boards:      boards,
		active:      make(map[int64]*scoreboard.Scoreboard),
		subscribers: make(map[int64]map[chan scoreboard.View]struct{}),
	}
}

// View returns the current state of a game's scoreboard.
func (s *ScoreboardService) View(ctx context.Context, id int64) (scoreboard.View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sb, err := s.load(ctx, id)
	if err != nil {
		return scoreboard.View{}, translate(err)
	}
	return sb.State().View(), nil
}

// Subscribe returns the current state of a game and a channel that receives
// every later change. Call cancel when the subscriber goes away.
func (s *ScoreboardService) Subscribe(ctx context.Context, id int64) (current scoreboard.View, updates <-chan scoreboard.View, cancel func(), err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sb, err := s.load(ctx, id)
	if err != nil {
		return scoreboard.View{}, nil, nil, translate(err)
	}

	channel := make(chan scoreboard.View, subscriberBuffer)
	if s.subscribers[id] == nil {
		s.subscribers[id] = make(map[chan scoreboard.View]struct{})
	}
	s.subscribers[id][channel] = struct{}{}

	cancel = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.subscribers[id], channel)
	}
	return sb.State().View(), channel, cancel, nil
}

func (s *ScoreboardService) Point(ctx context.Context, id int64, team int) (scoreboard.View, error) {
	return s.change(ctx, id, func(sb *scoreboard.Scoreboard) error { return sb.Point(team) })
}

func (s *ScoreboardService) AddGame(ctx context.Context, id int64, team int) (scoreboard.View, error) {
	return s.change(ctx, id, func(sb *scoreboard.Scoreboard) error { return sb.AddGame(team) })
}

func (s *ScoreboardService) RemoveGame(ctx context.Context, id int64, team int) (scoreboard.View, error) {
	return s.change(ctx, id, func(sb *scoreboard.Scoreboard) error { return sb.RemoveGame(team) })
}

func (s *ScoreboardService) AddSet(ctx context.Context, id int64, team int) (scoreboard.View, error) {
	return s.change(ctx, id, func(sb *scoreboard.Scoreboard) error { return sb.AddSet(team) })
}

func (s *ScoreboardService) RemoveSet(ctx context.Context, id int64, team int) (scoreboard.View, error) {
	return s.change(ctx, id, func(sb *scoreboard.Scoreboard) error { return sb.RemoveSet(team) })
}

func (s *ScoreboardService) SwitchServe(ctx context.Context, id int64) (scoreboard.View, error) {
	return s.change(ctx, id, func(sb *scoreboard.Scoreboard) error {
		sb.SwitchServe()
		return nil
	})
}

func (s *ScoreboardService) Undo(ctx context.Context, id int64) (scoreboard.View, error) {
	return s.change(ctx, id, func(sb *scoreboard.Scoreboard) error {
		if !sb.Undo() {
			return errors.New("nothing to undo")
		}
		return nil
	})
}

func (s *ScoreboardService) SetTeams(ctx context.Context, id int64, teams [2]scoreboard.Team) (scoreboard.View, error) {
	return s.change(ctx, id, func(sb *scoreboard.Scoreboard) error {
		sb.SetTeams(teams)
		return nil
	})
}

func (s *ScoreboardService) Reset(ctx context.Context, id int64, bestOf int) (scoreboard.View, error) {
	return s.change(ctx, id, func(sb *scoreboard.Scoreboard) error {
		return sb.Reset(bestOf)
	})
}

// change applies a rule to a game's scoreboard. It runs only if the game is open,
// saves the result, and then notifies the subscribers of that game.
func (s *ScoreboardService) change(ctx context.Context, id int64, rule func(*scoreboard.Scoreboard) error) (scoreboard.View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	game, err := s.games.FindByID(ctx, id)
	if err != nil {
		return scoreboard.View{}, translate(err)
	}
	if game.FinishedAt != nil {
		return scoreboard.View{}, ValidationError{ErrGameFinished}
	}

	sb, err := s.load(ctx, id)
	if err != nil {
		return scoreboard.View{}, translate(err)
	}
	if err := rule(sb); err != nil {
		return scoreboard.View{}, ValidationError{err}
	}
	if err := s.boards.SaveState(ctx, id, sb.State().Snapshot()); err != nil {
		// Drop the cached copy so the next call reloads what is saved.
		delete(s.active, id)
		return scoreboard.View{}, translate(err)
	}

	view := sb.State().View()
	s.notify(id, view)
	return view, nil
}

// load returns the in-memory scoreboard of a game, loading it from the database on first use.
// Must be called with s.mu held.
func (s *ScoreboardService) load(ctx context.Context, id int64) (*scoreboard.Scoreboard, error) {
	if sb, ok := s.active[id]; ok {
		return sb, nil
	}
	saved, err := s.boards.FindByGameID(ctx, id)
	if err != nil {
		return nil, err
	}
	match, err := scoreboard.RestoreMatch(saved.State)
	if err != nil {
		return nil, err
	}
	sb := scoreboard.NewScoreboard(match)
	s.active[id] = sb
	return sb, nil
}

// notify sends a view to every subscriber of a game. If a subscriber's buffer is full,
// its oldest update is dropped: only the latest state matters. Must be called with s.mu held.
func (s *ScoreboardService) notify(id int64, view scoreboard.View) {
	for channel := range s.subscribers[id] {
		select {
		case channel <- view:
		default:
			<-channel
			channel <- view
		}
	}
}
