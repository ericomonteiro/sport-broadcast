// Package service holds the application rules: creating and finishing games,
// changing live scoreboards, and notifying subscribers. It knows nothing about HTTP.
package service

import (
	"errors"

	"broadcast/internal/repository"
)

// ErrNotFound means the game does not exist.
var ErrNotFound = errors.New("game not found")

// ErrGameFinished means the game was finished and no longer accepts changes.
var ErrGameFinished = errors.New("match is finished")

// ValidationError is a rejected request: bad input or a rule violation.
// Its message is safe to show to the operator.
type ValidationError struct {
	Err error
}

func (e ValidationError) Error() string { return e.Err.Error() }

func (e ValidationError) Unwrap() error { return e.Err }

// translate turns repository errors into service errors, so callers never see the repository package.
// Stores report a missing row with repository.ErrNotFound.
func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
