// Package model holds the entities stored in the database.
package model

import (
	"time"

	"broadcast/internal/scoreboard"
)

// Game is one match being broadcast. Its ID is the one used in the overlay URL.
// FinishedAt is nil while the match is open.
type Game struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

// Scoreboard is the saved score of a game. Each game has exactly one.
type Scoreboard struct {
	ID        int64               `json:"id"`
	GameID    int64               `json:"gameId"`
	State     scoreboard.Snapshot `json:"state"`
	UpdatedAt time.Time           `json:"updatedAt"`
}
