package repository

import (
	"context"
	"encoding/json"

	"broadcast/internal/model"
	"broadcast/internal/scoreboard"
)

// FindByGameID returns the saved scoreboard of a game, or ErrNotFound.
func (r *Scoreboards) FindByGameID(ctx context.Context, gameID int64) (model.Scoreboard, error) {
	var (
		board model.Scoreboard
		raw   []byte
	)
	err := r.db.QueryRow(ctx,
		`SELECT id, game_id, state, updated_at FROM scoreboards WHERE game_id = $1`, gameID,
	).Scan(&board.ID, &board.GameID, &raw, &board.UpdatedAt)
	if err != nil {
		return model.Scoreboard{}, wrap("finding scoreboard", mapNotFound(err))
	}

	if err := json.Unmarshal(raw, &board.State); err != nil {
		return model.Scoreboard{}, wrap("decoding scoreboard state", err)
	}
	return board, nil
}

// SaveState overwrites the saved state of a game's scoreboard.
func (r *Scoreboards) SaveState(ctx context.Context, gameID int64, state scoreboard.Snapshot) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return wrap("encoding scoreboard state", err)
	}

	tag, err := r.db.Exec(ctx,
		`UPDATE scoreboards SET state = $2, updated_at = now() WHERE game_id = $1`, gameID, raw)
	if err != nil {
		return wrap("saving scoreboard", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
