// Package repository runs the queries for games and scoreboards.
package repository

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound means the requested row does not exist.
var ErrNotFound = errors.New("not found")

// Games stores and reads games.
type Games struct {
	db *pgxpool.Pool
}

// NewGames creates a Games repository.
func NewGames(db *pgxpool.Pool) *Games {
	return &Games{db: db}
}

// Scoreboards stores and reads the scoreboard of each game.
type Scoreboards struct {
	db *pgxpool.Pool
}

// NewScoreboards creates a Scoreboards repository.
func NewScoreboards(db *pgxpool.Pool) *Scoreboards {
	return &Scoreboards{db: db}
}

// mapNotFound turns pgx's "no rows" error into ErrNotFound.
func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func wrap(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}
