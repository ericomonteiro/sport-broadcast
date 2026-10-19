package service

import (
	"context"
	"errors"
	"testing"

	"broadcast/internal/scoreboard"
)

// fakeReplay records how many times a replay was requested and can fail on demand.
type fakeReplay struct {
	calls int
	err   error
}

func (f *fakeReplay) SaveReplay() error {
	f.calls++
	return f.err
}

func TestReplaySaveAsksOBSForAnExistingGame(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	game, _ := store.Create(ctx, "Final", scoreboard.Snapshot{})
	obs := &fakeReplay{}

	if err := NewReplayService(store, obs).Save(ctx, game.ID); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if obs.calls != 1 {
		t.Fatalf("expected one replay request, got %d", obs.calls)
	}
}

func TestReplaySaveRejectsUnknownGameWithoutCallingOBS(t *testing.T) {
	obs := &fakeReplay{}

	err := NewReplayService(newMemoryStore(), obs).Save(context.Background(), 99)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if obs.calls != 0 {
		t.Fatalf("OBS must not be called for an unknown game")
	}
}

func TestReplaySaveReportsOBSFailureAsUnavailable(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	game, _ := store.Create(ctx, "Final", scoreboard.Snapshot{})
	obs := &fakeReplay{err: errors.New("connection refused")}

	err := NewReplayService(store, obs).Save(ctx, game.ID)
	if !errors.Is(err, ErrReplayUnavailable) {
		t.Fatalf("expected ErrReplayUnavailable, got %v", err)
	}
}
