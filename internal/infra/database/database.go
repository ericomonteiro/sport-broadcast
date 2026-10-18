// Package database manages the Postgres connection pool and the schema.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schema is applied on every start. Each statement is idempotent.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS games (
		id         BIGSERIAL PRIMARY KEY,
		name       TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`ALTER TABLE games ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ`,
	`CREATE TABLE IF NOT EXISTS scoreboards (
		id         BIGSERIAL PRIMARY KEY,
		game_id    BIGINT NOT NULL UNIQUE REFERENCES games(id) ON DELETE CASCADE,
		state      JSONB NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
}

// Connect opens a connection pool and checks that the database answers.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	return pool, nil
}

// Migrate creates the tables if they do not exist yet.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	for _, statement := range schema {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("applying schema: %w", err)
		}
	}
	return nil
}
