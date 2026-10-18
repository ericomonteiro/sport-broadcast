# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

Go 1.24, standard library only (no external dependencies). Git repository on branch `master`, no commits yet. No linter config exists; use `gofmt` and `go vet`.

```bash
go run ./cmd/broadcast -addr :8080 -best-of 3   # start the server
go build ./cmd/broadcast                        # build the binary
go test ./...                                   # run all tests
go test ./internal/scoreboard -run TestUndo     # run a single test
gofmt -l -w .                                   # format
go vet ./...                                    # static checks
```

## Architecture

The app is a live scoreboard for beach tennis broadcasts. An operator changes the score in a control page; OBS shows an overlay page through a Browser Source, and the overlay updates in real time.

Three layers:

- **`internal/scoreboard`**: pure scoring rules, no I/O. `Match` is the state, and it only uses fixed-size arrays so it can be copied by value. `Scoreboard` wraps the current `Match` plus a history of snapshots, which powers undo. The rules (games, sets, tie-break, best-of) are hardcoded here and documented in the package comment. `Match.View()` converts the state into display strings (`"0"`, `"15"`, `"40"`, `"Vant."`) and is the JSON shape sent to the browser.
- **`internal/server`**: HTTP layer. A single `sync.Mutex` guards the `Scoreboard`. Every successful POST calls `broadcast()`, which pushes the new `View` to every SSE client (`/events`). Each client has a buffered channel of size 8; if it is full, the oldest message is dropped, because only the latest state matters. The `/events` handler sends the current state immediately on connect, so a reconnecting page is always in sync.
- **`web/`**: HTML pages embedded in the binary with `embed.FS`. `overlay.html` (transparent background, meant for OBS) and `control.html` (operator panel). Both pages render the same `View` JSON and listen to `/events`.

The `cmd/broadcast/main.go` entry point wires the pieces: `NewMatch` → `NewScoreboard` → `server.New` → `ListenAndServe`.

### Things that are easy to get wrong

- **JSON field names are a contract with the HTML.** Renaming a `View` field (`json` tag in `scoreboard.go`) requires updating the JavaScript in `overlay.html` and `control.html`.
- **Undo does not restore team names.** Names are not part of the history; `Undo()` keeps the current `Teams`. `Reset()` keeps names too.
- **Serve is manual.** `SwitchServe()` is never automatic, because the rotation rules are not implemented.
- **Routes:** `GET /scoreboard/overlay` (OBS source), `GET /control`, `GET /state`, `GET /events`, and `POST /api/point`, `/api/serve`, `/api/undo`, `/api/teams`, `/api/reset`. Errors are `400` for invalid JSON and `422` for rule violations.

## Conventions

- Code identifiers, comments, and error messages are in English.
- Text shown to operators and on the broadcast (button labels, "Trocar saque", "Partida encerrada") stays in Portuguese.
- The module is named `broadcast`; the project folder is `transmissao`.
