package repository

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"broadcast/internal/model"
	"broadcast/internal/scoreboard"
)

const gameColumns = `id, name, created_at, finished_at`

func scanGame(row pgx.Row) (model.Game, error) {
	var game model.Game
	err := row.Scan(&game.ID, &game.Name, &game.CreatedAt, &game.FinishedAt)
	return game, err
}

// Create stores a game together with its initial scoreboard, in one transaction.
// Either both rows exist afterwards or neither does.
func (r *Games) Create(ctx context.Context, name string, initial scoreboard.Snapshot) (model.Game, error) {
	state, err := json.Marshal(initial)
	if err != nil {
		return model.Game{}, wrap("encoding scoreboard", err)
	}

	var game model.Game
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		created, err := scanGame(tx.QueryRow(ctx,
			`INSERT INTO games (name) VALUES ($1) RETURNING `+gameColumns, name))
		if err != nil {
			return err
		}
		game = created
		_, err = tx.Exec(ctx,
			`INSERT INTO scoreboards (game_id, state) VALUES ($1, $2)`, game.ID, state)
		return err
	})
	if err != nil {
		return model.Game{}, wrap("creating game", err)
	}
	return game, nil
}

// FindByID returns the game with the given ID, or ErrNotFound.
func (r *Games) FindByID(ctx context.Context, id int64) (model.Game, error) {
	game, err := scanGame(r.db.QueryRow(ctx,
		`SELECT `+gameColumns+` FROM games WHERE id = $1`, id))
	if err != nil {
		return model.Game{}, wrap("finding game", mapNotFound(err))
	}
	return game, nil
}

// List returns all games, oldest first.
func (r *Games) List(ctx context.Context) ([]model.Game, error) {
	return r.query(ctx, `SELECT `+gameColumns+` FROM games ORDER BY id`)
}

// ListOpen returns the games that are not finished yet, oldest first.
func (r *Games) ListOpen(ctx context.Context) ([]model.Game, error) {
	return r.query(ctx, `SELECT `+gameColumns+` FROM games WHERE finished_at IS NULL ORDER BY id`)
}

// Finish marks a game as finished. Finishing a finished game changes nothing.
func (r *Games) Finish(ctx context.Context, id int64) (model.Game, error) {
	game, err := scanGame(r.db.QueryRow(ctx,
		`UPDATE games SET finished_at = COALESCE(finished_at, now())
		 WHERE id = $1 RETURNING `+gameColumns, id))
	if err != nil {
		return model.Game{}, wrap("finishing game", mapNotFound(err))
	}
	return game, nil
}

func (r *Games) query(ctx context.Context, sql string) ([]model.Game, error) {
	rows, err := r.db.Query(ctx, sql)
	if err != nil {
		return nil, wrap("listing games", err)
	}
	defer rows.Close()

	games := []model.Game{}
	for rows.Next() {
		game, err := scanGame(rows)
		if err != nil {
			return nil, wrap("reading game", err)
		}
		games = append(games, game)
	}
	return games, wrap("listing games", rows.Err())
}
