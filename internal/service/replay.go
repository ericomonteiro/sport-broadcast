package service

import (
	"context"
	"errors"
	"fmt"
)

// ErrReplayUnavailable means the replay could not be saved: OBS is closed, its
// WebSocket server is off, or the replay buffer is not running.
var ErrReplayUnavailable = errors.New("OBS indisponível: verifique se o WebSocket está ativo e o replay buffer iniciado")

// ReplayService saves the instant replay of a game. A replay does not change the
// score, so it is allowed even after the game is finished.
type ReplayService struct {
	games  GameStore
	replay ReplaySaver
}

// NewReplayService creates a ReplayService.
func NewReplayService(games GameStore, replay ReplaySaver) *ReplayService {
	return &ReplayService{games: games, replay: replay}
}

// Save asks OBS to save the replay buffer for a game.
func (s *ReplayService) Save(ctx context.Context, id int64) error {
	if _, err := s.games.FindByID(ctx, id); err != nil {
		return translate(err)
	}
	if err := s.replay.SaveReplay(); err != nil {
		return fmt.Errorf("%w: %v", ErrReplayUnavailable, err)
	}
	return nil
}
