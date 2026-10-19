// Package handler is the HTTP layer. It parses requests, calls the services,
// and turns results into responses. It holds no business rules.
//
// Routes:
//   - pages (OBS overlay and operator panel) for one game
//   - game list, creation, lookup and finish
//   - per-game state, SSE stream, and actions on the scoreboard
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"broadcast/internal/scoreboard"
	"broadcast/internal/service"
	"broadcast/resource"
	"broadcast/web"
)

type Handler struct {
	games       *service.GameService
	scoreboards *service.ScoreboardService
	replays     *service.ReplayService
}

// New creates a Handler on top of the services.
func New(games *service.GameService, scoreboards *service.ScoreboardService, replays *service.ReplayService) *Handler {
	return &Handler{games: games, scoreboards: scoreboards, replays: replays}
}

// Routes returns the HTTP router with all routes.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, web.Files, "index.html")
	})
	icons, err := fs.Sub(resource.Icons, "icons")
	if err != nil {
		panic(err) // the icons directory is embedded at build time; this cannot fail at runtime
	}
	mux.Handle("GET /icons/", http.StripPrefix("/icons/", http.FileServerFS(icons)))

	mux.HandleFunc("GET /scoreboard/{id}/overlay", h.page("overlay.html"))
	mux.HandleFunc("GET /scoreboard/{id}/control", h.page("control.html"))

	mux.HandleFunc("GET /api/games", h.listGames)
	mux.HandleFunc("POST /api/games", h.createGame)
	mux.HandleFunc("GET /api/games/{id}", h.getGame)
	mux.HandleFunc("POST /api/games/{id}/finish", h.finishGame)
	mux.HandleFunc("POST /api/games/{id}/replay", h.saveReplay)

	mux.HandleFunc("GET /api/scoreboard/{id}/state", h.state)
	mux.HandleFunc("GET /api/scoreboard/{id}/events", h.events)

	mux.HandleFunc("POST /api/scoreboard/{id}/point", h.action(func(ctx context.Context, id int64, b requestBody) (scoreboard.View, error) {
		return h.scoreboards.Point(ctx, id, b.Team)
	}))
	mux.HandleFunc("POST /api/scoreboard/{id}/games/add", h.action(func(ctx context.Context, id int64, b requestBody) (scoreboard.View, error) {
		return h.scoreboards.AddGame(ctx, id, b.Team)
	}))
	mux.HandleFunc("POST /api/scoreboard/{id}/games/remove", h.action(func(ctx context.Context, id int64, b requestBody) (scoreboard.View, error) {
		return h.scoreboards.RemoveGame(ctx, id, b.Team)
	}))
	mux.HandleFunc("POST /api/scoreboard/{id}/sets/add", h.action(func(ctx context.Context, id int64, b requestBody) (scoreboard.View, error) {
		return h.scoreboards.AddSet(ctx, id, b.Team)
	}))
	mux.HandleFunc("POST /api/scoreboard/{id}/sets/remove", h.action(func(ctx context.Context, id int64, b requestBody) (scoreboard.View, error) {
		return h.scoreboards.RemoveSet(ctx, id, b.Team)
	}))
	mux.HandleFunc("POST /api/scoreboard/{id}/serve", h.action(func(ctx context.Context, id int64, _ requestBody) (scoreboard.View, error) {
		return h.scoreboards.SwitchServe(ctx, id)
	}))
	mux.HandleFunc("POST /api/scoreboard/{id}/undo", h.action(func(ctx context.Context, id int64, _ requestBody) (scoreboard.View, error) {
		return h.scoreboards.Undo(ctx, id)
	}))
	mux.HandleFunc("POST /api/scoreboard/{id}/teams", h.action(func(ctx context.Context, id int64, b requestBody) (scoreboard.View, error) {
		return h.scoreboards.SetTeams(ctx, id, b.Teams)
	}))
	mux.HandleFunc("POST /api/scoreboard/{id}/reset", h.action(func(ctx context.Context, id int64, b requestBody) (scoreboard.View, error) {
		return h.scoreboards.Reset(ctx, id, b.BestOf)
	}))

	return mux
}

// requestBody is the JSON accepted by the POST routes. Each route only uses the fields it needs.
type requestBody struct {
	Name   string             `json:"name"`
	Team   int                `json:"team"`
	Teams  [2]scoreboard.Team `json:"teams"`
	BestOf int                `json:"bestOf"`
}

// action runs one scoreboard operation and responds with the resulting state.
type action func(ctx context.Context, id int64, b requestBody) (scoreboard.View, error)

// page serves one of the HTML files, if the game exists.
func (h *Handler) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		if _, err := h.games.Get(r.Context(), id); err != nil {
			fail(w, err)
			return
		}
		http.ServeFileFS(w, r, web.Files, name)
	}
}

func (h *Handler) listGames(w http.ResponseWriter, r *http.Request) {
	games, err := h.games.List(r.Context(), r.URL.Query().Get("status") == "open")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, games)
}

func (h *Handler) getGame(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	game, err := h.games.Get(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, game)
}

func (h *Handler) createGame(w http.ResponseWriter, r *http.Request) {
	var b requestBody
	if !decodeBody(w, r, &b) {
		return
	}
	game, err := h.games.Create(r.Context(), service.NewGame{
		Name:   b.Name,
		BestOf: b.BestOf,
		Teams:  b.Teams,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, game)
}

func (h *Handler) finishGame(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	game, err := h.games.Finish(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, game)
}

func (h *Handler) saveReplay(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.replays.Save(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) state(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	view, err := h.scoreboards.View(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) action(run action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		var b requestBody
		if !decodeBody(w, r, &b) {
			return
		}
		view, err := run(r.Context(), id, b)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

// events opens an SSE stream for one game. The client gets the current state as soon as it connects.
func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	current, updates, cancel, err := h.scoreboards.Subscribe(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if !writeEvent(w, flusher, current) {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case view := <-updates:
			if !writeEvent(w, flusher, view) {
				return
			}
		}
	}
}

// writeEvent sends one SSE message. It returns false if the client is gone.
func writeEvent(w http.ResponseWriter, flusher http.Flusher, view scoreboard.View) bool {
	data, err := json.Marshal(view)
	if err != nil {
		log.Printf("error serializing state: %v", err)
		return false
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid game id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func decodeBody(w http.ResponseWriter, r *http.Request, b *requestBody) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(b); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

// fail maps service errors to HTTP responses. Unexpected errors are logged, not shown to the client.
func fail(w http.ResponseWriter, err error) {
	var validation service.ValidationError
	switch {
	case errors.As(err, &validation):
		http.Error(w, validation.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, service.ErrNotFound):
		http.Error(w, service.ErrNotFound.Error(), http.StatusNotFound)
	case errors.Is(err, service.ErrReplayUnavailable):
		log.Printf("replay: %v", err)
		http.Error(w, service.ErrReplayUnavailable.Error(), http.StatusServiceUnavailable)
	default:
		log.Printf("internal error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error writing response: %v", err)
	}
}
