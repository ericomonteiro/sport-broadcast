package service

import (
	"context"
	"errors"
	"strings"

	"broadcast/internal/model"
	"broadcast/internal/scoreboard"
)

// GameService manages the lifecycle of games.
type GameService struct {
	games GameStore
}

// NewGameService creates a GameService.
func NewGameService(games GameStore) *GameService {
	return &GameService{games: games}
}

// NewGame is the input to create a game.
type NewGame struct {
	Name   string
	BestOf int
	Teams  [2]scoreboard.Team
}

// Create validates the input and stores a new game with an empty scoreboard.
func (s *GameService) Create(ctx context.Context, in NewGame) (model.Game, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return model.Game{}, ValidationError{errors.New("name is required")}
	}
	match, err := scoreboard.NewMatch(in.Teams, in.BestOf)
	if err != nil {
		return model.Game{}, ValidationError{err}
	}

	game, err := s.games.Create(ctx, name, match.Snapshot())
	return game, translate(err)
}

// List returns all games, or only the open ones when openOnly is set.
func (s *GameService) List(ctx context.Context, openOnly bool) ([]model.Game, error) {
	var (
		games []model.Game
		err   error
	)
	if openOnly {
		games, err = s.games.ListOpen(ctx)
	} else {
		games, err = s.games.List(ctx)
	}
	return games, translate(err)
}

// Get returns one game.
func (s *GameService) Get(ctx context.Context, id int64) (model.Game, error) {
	game, err := s.games.FindByID(ctx, id)
	return game, translate(err)
}

// Finish closes a game. Finishing a finished game changes nothing.
func (s *GameService) Finish(ctx context.Context, id int64) (model.Game, error) {
	game, err := s.games.Finish(ctx, id)
	return game, translate(err)
}
