package service

import (
	"context"

	"broadcast/internal/model"
	"broadcast/internal/scoreboard"
)

// GameStore is what the services need to read and write games.
// repository.Games implements it; tests use an in-memory fake.
type GameStore interface {
	Create(ctx context.Context, name string, initial scoreboard.Snapshot) (model.Game, error)
	FindByID(ctx context.Context, id int64) (model.Game, error)
	List(ctx context.Context) ([]model.Game, error)
	ListOpen(ctx context.Context) ([]model.Game, error)
	Finish(ctx context.Context, id int64) (model.Game, error)
}

// ScoreboardStore is what the services need to load and save a game's scoreboard.
// repository.Scoreboards implements it; tests use an in-memory fake.
type ScoreboardStore interface {
	FindByGameID(ctx context.Context, gameID int64) (model.Scoreboard, error)
	SaveState(ctx context.Context, gameID int64, state scoreboard.Snapshot) error
}

// ReplaySaver asks the broadcast software to save its replay buffer.
// The OBS adapter (internal/infra/obs) implements it; tests use a fake.
type ReplaySaver interface {
	SaveReplay(seconds int) error
}
