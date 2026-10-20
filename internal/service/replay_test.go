package service

import (
	"context"
	"errors"
	"testing"

	"broadcast/internal/scoreboard"
)

// fakeReplay records how many times a replay was requested and can fail on demand.
type fakeReplay struct {
	calls   int
	seconds int
	err     error
}

func (f *fakeReplay) SaveReplay(seconds int) error {
	f.calls++
	f.seconds = seconds
	return f.err
}

func TestReplaySaveAsksOBSForAnExistingGame(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	game, _ := store.Create(ctx, "Final", scoreboard.Snapshot{})
	obs := &fakeReplay{}

	if err := NewReplayService(store, obs).Save(ctx, game.ID, 15); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if obs.calls != 1 || obs.seconds != 15 {
		t.Fatalf("expected one 15 s replay request, got %d calls for %d s", obs.calls, obs.seconds)
	}
}

func TestReplaySaveRejectsUnknownGameWithoutCallingOBS(t *testing.T) {
	obs := &fakeReplay{}

	err := NewReplayService(newMemoryStore(), obs).Save(context.Background(), 99, 10)
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

	err := NewReplayService(store, obs).Save(ctx, game.ID, 10)
	if !errors.Is(err, ErrReplayUnavailable) {
		t.Fatalf("expected ErrReplayUnavailable, got %v", err)
	}
}

func TestReplaySaveRejectsLengthOutsideTheChoices(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	game, _ := store.Create(ctx, "Final", scoreboard.Snapshot{})
	obs := &fakeReplay{}

	for _, seconds := range []int{0, 3, 12, 30} {
		err := NewReplayService(store, obs).Save(ctx, game.ID, seconds)
		var validation ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("seconds %d: expected ValidationError, got %v", seconds, err)
		}
	}
	if obs.calls != 0 {
		t.Fatalf("OBS must not be called for an invalid length")
	}
}
