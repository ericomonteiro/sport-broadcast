// Package server exposes the scoreboard over HTTP: the overlay and control
// pages, an API to register plays, and an SSE stream that pushes every change
// to open pages (including the OBS Browser Source).
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"

	"broadcast/internal/scoreboard"
	"broadcast/web"
)

type Server struct {
	mu         sync.Mutex
	scoreboard *scoreboard.Scoreboard
	clients    map[chan []byte]struct{}
}

// New creates a server around a configured scoreboard.
func New(sb *scoreboard.Scoreboard) *Server {
	return &Server{
		scoreboard: sb,
		clients:    make(map[chan []byte]struct{}),
	}
}

// Handler returns the HTTP router with all routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /scoreboard/overlay", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, web.Files, "overlay.html")
	})
	mux.HandleFunc("GET /control", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, web.Files, "control.html")
	})

	mux.HandleFunc("GET /state", s.state)
	mux.HandleFunc("GET /events", s.events)

	mux.HandleFunc("POST /api/point", s.withBody(func(w http.ResponseWriter, b requestBody) {
		s.apply(w, func() error { return s.scoreboard.Point(b.Team) })
	}))
	mux.HandleFunc("POST /api/serve", s.withBody(func(w http.ResponseWriter, _ requestBody) {
		s.apply(w, func() error {
			s.scoreboard.SwitchServe()
			return nil
		})
	}))
	mux.HandleFunc("POST /api/undo", func(w http.ResponseWriter, r *http.Request) {
		s.apply(w, func() error {
			if !s.scoreboard.Undo() {
				return errors.New("nothing to undo")
			}
			return nil
		})
	})
	mux.HandleFunc("POST /api/teams", s.withBody(func(w http.ResponseWriter, b requestBody) {
		s.apply(w, func() error {
			s.scoreboard.SetTeams(b.Teams)
			return nil
		})
	}))
	mux.HandleFunc("POST /api/reset", s.withBody(func(w http.ResponseWriter, b requestBody) {
		s.apply(w, func() error { return s.scoreboard.Reset(b.BestOf) })
	}))

	return mux
}

// requestBody is the JSON accepted by the POST routes. Each route only uses the fields it needs.
type requestBody struct {
	Team   int                `json:"team"`
	Teams  [2]scoreboard.Team `json:"teams"`
	BestOf int                `json:"bestOf"`
}

func (s *Server) withBody(handle func(http.ResponseWriter, requestBody)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var b requestBody
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
		}
		handle(w, b)
	}
}

// apply runs the change, notifies clients and responds with the new state.
func (s *Server) apply(w http.ResponseWriter, change func() error) {
	s.mu.Lock()
	err := change()
	if err == nil {
		s.broadcast()
	}
	view := s.scoreboard.State().View()
	s.mu.Unlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, view)
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	view := s.scoreboard.State().View()
	s.mu.Unlock()
	writeJSON(w, view)
}

// events opens an SSE stream. The client receives the current state as soon as it connects.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	channel := make(chan []byte, 8)

	s.mu.Lock()
	s.clients[channel] = struct{}{}
	initial, _ := json.Marshal(s.scoreboard.State().View())
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, channel)
		s.mu.Unlock()
	}()

	fmt.Fprintf(w, "data: %s\n\n", initial)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-channel:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// broadcast sends the current state to every client. Must be called with s.mu held.
// If a client's channel is full, the oldest message is dropped: only the latest state matters.
func (s *Server) broadcast() {
	data, err := json.Marshal(s.scoreboard.State().View())
	if err != nil {
		log.Printf("error serializing state: %v", err)
		return
	}
	for channel := range s.clients {
		select {
		case channel <- data:
		default:
			<-channel
			channel <- data
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error writing response: %v", err)
	}
}
