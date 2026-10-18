package service

import (
	"context"
	"sync"
	"time"

	"broadcast/internal/model"
	"broadcast/internal/repository"
	"broadcast/internal/scoreboard"
)

// memoryStore is an in-memory fake of GameStore and ScoreboardStore.
// It keeps the same contract as the repositories: missing rows are repository.ErrNotFound.
type memoryStore struct {
	mu     sync.Mutex
	nextID int64
	games  map[int64]model.Game
	boards map[int64]scoreboard.Snapshot
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		nextID: 1,
		games:  make(map[int64]model.Game),
		boards: make(map[int64]scoreboard.Snapshot),
	}
}

func (m *memoryStore) Create(_ context.Context, name string, initial scoreboard.Snapshot) (model.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	game := model.Game{ID: m.nextID, Name: name, CreatedAt: time.Now()}
	m.nextID++
	m.games[game.ID] = game
	m.boards[game.ID] = initial
	return game, nil
}

func (m *memoryStore) FindByID(_ context.Context, id int64) (model.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	game, ok := m.games[id]
	if !ok {
		return model.Game{}, repository.ErrNotFound
	}
	return game, nil
}

func (m *memoryStore) List(_ context.Context) ([]model.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sorted(func(model.Game) bool { return true }), nil
}

func (m *memoryStore) ListOpen(_ context.Context) ([]model.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sorted(func(g model.Game) bool { return g.FinishedAt == nil }), nil
}

func (m *memoryStore) Finish(_ context.Context, id int64) (model.Game, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	game, ok := m.games[id]
	if !ok {
		return model.Game{}, repository.ErrNotFound
	}
	if game.FinishedAt == nil {
		now := time.Now()
		game.FinishedAt = &now
		m.games[id] = game
	}
	return game, nil
}

func (m *memoryStore) FindByGameID(_ context.Context, gameID int64) (model.Scoreboard, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.boards[gameID]
	if !ok {
		return model.Scoreboard{}, repository.ErrNotFound
	}
	return model.Scoreboard{GameID: gameID, State: state}, nil
}

func (m *memoryStore) SaveState(_ context.Context, gameID int64, state scoreboard.Snapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.boards[gameID]; !ok {
		return repository.ErrNotFound
	}
	m.boards[gameID] = state
	return nil
}

// sorted returns the games matching keep, ordered by ID. Must be called with m.mu held.
func (m *memoryStore) sorted(keep func(model.Game) bool) []model.Game {
	games := []model.Game{}
	for id := int64(1); id < m.nextID; id++ {
		if game, ok := m.games[id]; ok && keep(game) {
			games = append(games, game)
		}
	}
	return games
}
