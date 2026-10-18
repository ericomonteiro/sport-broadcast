package service

import (
	"context"
	"errors"
	"testing"

	"broadcast/internal/scoreboard"
)

func newTestServices() (*GameService, *ScoreboardService) {
	store := newMemoryStore()
	return NewGameService(store), NewScoreboardService(store, store)
}

func newGame(t *testing.T, games *GameService) int64 {
	t.Helper()
	game, err := games.Create(context.Background(), NewGame{
		Name:   "Partida de teste",
		BestOf: 3,
		Teams: [2]scoreboard.Team{
			{Players: []string{"A"}},
			{Players: []string{"B"}},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return game.ID
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	games, _ := newTestServices()

	_, err := games.Create(context.Background(), NewGame{Name: "  ", BestOf: 3})
	var validation ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("empty name: expected ValidationError, got %v", err)
	}

	_, err = games.Create(context.Background(), NewGame{Name: "x", BestOf: 4})
	if !errors.As(err, &validation) || !errors.Is(err, scoreboard.ErrInvalidBestOf) {
		t.Fatalf("bestOf 4: expected ValidationError wrapping ErrInvalidBestOf, got %v", err)
	}
}

func TestCreateTrimsName(t *testing.T) {
	games, _ := newTestServices()

	game, err := games.Create(context.Background(), NewGame{Name: "  Final  ", BestOf: 1})
	if err != nil || game.Name != "Final" {
		t.Fatalf("expected trimmed name, got %+v, %v", game, err)
	}
}

func TestListOpenLeavesOutFinishedGames(t *testing.T) {
	games, _ := newTestServices()
	ctx := context.Background()
	open := newGame(t, games)
	done := newGame(t, games)

	if _, err := games.Finish(ctx, done); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	list, err := games.List(ctx, true)
	if err != nil || len(list) != 1 || list[0].ID != open {
		t.Fatalf("expected only the open game, got %+v, %v", list, err)
	}
	all, err := games.List(ctx, false)
	if err != nil || len(all) != 2 {
		t.Fatalf("expected both games, got %+v, %v", all, err)
	}
}

func TestChangesAreRejectedAfterFinish(t *testing.T) {
	games, scores := newTestServices()
	ctx := context.Background()
	id := newGame(t, games)

	if _, err := scores.Point(ctx, id, 0); err != nil {
		t.Fatalf("Point before finish: %v", err)
	}
	if _, err := games.Finish(ctx, id); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	_, err := scores.Point(ctx, id, 1)
	if !errors.Is(err, ErrGameFinished) {
		t.Fatalf("Point after finish: expected ErrGameFinished, got %v", err)
	}
	var validation ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("Point after finish: expected ValidationError, got %v", err)
	}

	view, err := scores.View(ctx, id)
	if err != nil || view.Points != [2]string{"15", "0"} {
		t.Fatalf("finished match should keep its score, got %+v, %v", view.Points, err)
	}
}

func TestChangesArePersisted(t *testing.T) {
	store := newMemoryStore()
	games := NewGameService(store)
	scores := NewScoreboardService(store, store)
	ctx := context.Background()
	id := newGame(t, games)

	if _, err := scores.AddGame(ctx, id, 0); err != nil {
		t.Fatalf("AddGame: %v", err)
	}

	// A new service over the same store must load the saved state, as after a restart.
	restarted := NewScoreboardService(store, store)
	view, err := restarted.View(ctx, id)
	if err != nil || view.Games != [2]int{1, 0} {
		t.Fatalf("expected saved games 1-0 after restart, got %+v, %v", view.Games, err)
	}
}

func TestSubscribersReceiveChanges(t *testing.T) {
	games, scores := newTestServices()
	ctx := context.Background()
	id := newGame(t, games)

	current, updates, cancel, err := scores.Subscribe(ctx, id)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()
	if current.Points != [2]string{"0", "0"} {
		t.Fatalf("expected initial state, got %v", current.Points)
	}

	if _, err := scores.Point(ctx, id, 1); err != nil {
		t.Fatalf("Point: %v", err)
	}
	view := <-updates
	if view.Points != [2]string{"0", "15"} {
		t.Fatalf("expected update 0-15, got %v", view.Points)
	}
}

func TestRejectedChangeIsNotBroadcast(t *testing.T) {
	games, scores := newTestServices()
	ctx := context.Background()
	id := newGame(t, games)

	_, updates, cancel, err := scores.Subscribe(ctx, id)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	if _, err := scores.RemoveSet(ctx, id, 0); err == nil {
		t.Fatalf("RemoveSet with no sets should fail")
	}
	select {
	case view := <-updates:
		t.Fatalf("rejected change should not be broadcast, got %+v", view)
	default:
	}
}

func TestNotFoundIsServiceError(t *testing.T) {
	games, scores := newTestServices()
	ctx := context.Background()

	if _, err := games.Get(ctx, -1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get: expected ErrNotFound, got %v", err)
	}
	if _, err := scores.View(ctx, -1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("View: expected ErrNotFound, got %v", err)
	}
}
