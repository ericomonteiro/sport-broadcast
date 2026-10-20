package service

import (
	"context"
	"errors"
	"fmt"
)

// ReplayLengths are the replay lengths the operator can choose, in seconds. The
// longest one is the whole OBS replay buffer.
var ReplayLengths = []int{5, 10, 15, 20}

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

// Save asks OBS to save the replay buffer for a game and play its last `seconds`.
func (s *ReplayService) Save(ctx context.Context, id int64, seconds int) error {
	if !isReplayLength(seconds) {
		return ValidationError{fmt.Errorf("duração do replay deve ser 5, 10, 15 ou 20 segundos")}
	}
	if _, err := s.games.FindByID(ctx, id); err != nil {
		return translate(err)
	}
	if err := s.replay.SaveReplay(seconds); err != nil {
		return fmt.Errorf("%w: %v", ErrReplayUnavailable, err)
	}
	return nil
}

func isReplayLength(seconds int) bool {
	for _, n := range ReplayLengths {
		if n == seconds {
			return true
		}
	}
	return false
}
