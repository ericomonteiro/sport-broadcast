# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

Go 1.25 (the `go` directive in `go.mod` is 1.25.0, required by pgx v5.11). External dependencies: `github.com/jackc/pgx/v5` (Postgres) and `github.com/andreykaipov/goobs` (OBS WebSocket). Postgres runs in Docker. Git repository on branch `master`. No linter config exists; use `gofmt` and `go vet`.

```bash
docker compose up -d                                  # start Postgres (localhost:5432)
ENVIRONMENT=local go run ./cmd/broadcast              # start the server (reads application-local.properties)
go build ./cmd/broadcast                              # build the binary
go test ./...                                         # all tests; they use in-memory fakes, no database needed
go test ./internal/scoreboard -run TestUndo           # run a single test
gofmt -l -w .                                         # format
go vet ./...                                          # static checks
```

`application-local.properties` is git-ignored; `application-local.properties.example` is the committed template.

## Architecture

The app is a live scoreboard for beach tennis broadcasts. Each game has an ID and its own overlay and control pages. An operator changes the score in the control page; OBS shows the overlay of that game through a Browser Source, and the overlay updates in real time. Scores are saved to Postgres.

Layers, from the inside out:

- **`internal/scoreboard`**: pure scoring rules, no I/O. `Match` is the state and only uses fixed-size arrays so it can be copied by value. `Scoreboard` wraps the current `Match` plus a history of snapshots, which powers undo. `Match.View()` converts the state into display strings and is the JSON the browser receives. `Match.Snapshot()` / `RestoreMatch()` convert to and from the storable `Snapshot` (the only persistence path, because `Match` has private fields).
- **`internal/config`**: loads settings. The file is `application-<ENVIRONMENT>.properties` (`ENVIRONMENT=local` → `application-local.properties`). Environment variables override file values, using the key in upper case with `_` (`db.host` → `DB_HOST`). `DatabaseURL()` builds the Postgres URL.
- **`internal/infra/database`**: connection pool (`pgxpool`) and schema. `Migrate` runs the `CREATE TABLE IF NOT EXISTS` statements on every start; there is no migration tool.
- **`internal/infra/obs`**: OBS WebSocket client (obs-websocket 5.x, `goobs`). Connects on first use, reconnects after a failure. Config: `obs.addr` (default `localhost:4455`), `obs.password` (prefer the `OBS_PASSWORD` env var), `obs.replay.scene` (default `Replay`) and `obs.replay.source` (default `Replay Clip`), `obs.replay.intro` (default `Replay Intro`, plays before each clip), `obs.replay.live` (default `Live`, plays after each clip to return to the live picture). `Run` keeps the connection and reconnects. When OBS saves a replay (`ReplayBufferSaved`), the clip and live sources are hidden until their turn (they have "restart when active", so they would start as soon as the scene goes on air); the intro plays to its end and is then hidden; the clip plays next, then the live source plays to its end and is hidden, and the previous scene comes back (each stop is polled with `GetMediaInputStatus`).
- **`internal/model`**: entities. `Game` (ID is the one in the URLs; `FinishedAt` nil means open) and `Scoreboard` (one per game, FK `game_id`, holds the `Snapshot` as JSONB).
- **`internal/repository`**: queries (only the services call it). `Games.Create` inserts a game and its scoreboard in one transaction. `ErrNotFound` is the only sentinel; wrapped errors must be checked with `errors.Is`.
- **`internal/service`**: application rules, no HTTP. It depends on the `GameStore` and `ScoreboardStore` interfaces (`store.go`), not on the concrete repositories. `GameService` creates, lists, finishes games (validates input, returns `ValidationError` for bad input). `ScoreboardService` holds the live scoreboards: each game is loaded from the DB on first use and kept in memory; `change()` checks the game is not finished, applies the rule, saves with `SaveState` (on failure it drops the cached copy so the next call reloads), then notifies subscribers. Subscribers get a buffered channel (size 8; oldest dropped when full). A single `sync.Mutex` guards the cache and subscribers. Service errors: `ErrNotFound`, `ErrGameFinished`, `ValidationError` (its message is shown to the operator).
- **`internal/handler`**: HTTP only. Parses requests, calls the services, maps errors to status codes (`ValidationError` → 422, `ErrNotFound` → 404, anything else → 500, logged), writes JSON and SSE. Holds no business rules; a new rule goes in `service`.
- **`resource/`**: static assets embedded with `embed.FS` (`resource/icons/*.svg`), served under `GET /icons/`. The overlay uses `/icons/tennis.svg` as the serving marker.
- **`web/`**: HTML pages embedded with `embed.FS`. `index.html` (create/select/finish matches), `overlay.html` (transparent background, for OBS) and `control.html`. Both read the game ID from the path (`location.pathname.split("/")[2]`) and build the API base `/api/scoreboard/<id>`.

`cmd/broadcast/main.go` wires it: `config.Load` → `database.Connect` → `database.Migrate` → `repository.New*` → `service.New*` → `handler.New` → `ListenAndServe(h.Routes())`.

### Things that are easy to get wrong

- **JSON field names are a contract with the HTML.** Renaming a `View` or `Snapshot` field (`json` tags) requires updating `overlay.html` and `control.html`. Snapshot field names are also stored in the JSONB column, so renaming them breaks existing rows.
- **Finished games reject changes.** `finished_at` is set by `POST /api/games/{id}/finish` (idempotent). `ScoreboardService.change()` checks it on every change and returns `422 match is finished`. The overlay of a finished game still loads.
- **Undo history is in memory only.** It is not stored, so it is lost on restart (the saved score is kept). `Undo()` keeps the current team names.
- **Replay saves the OBS replay buffer.** `POST /api/games/{id}/replay` with `{"seconds": 5|10|15|20}` calls `SaveReplayBuffer`; the buffer length is set in OBS (Settings > Output > Replay Buffer, 20s). The requested length is queued in order and matched to the next `ReplayBufferSaved` event. Events without a request (OBS hotkey) play the full clip. For lengths under 20s the clip is seeked to `duration - seconds` (once it is playing; a single seek, never retried, because repeated seeks made the clip jump back). It works after the match is finished. If OBS is closed or the buffer is off, the response is `503`.
- **Serve is manual.** `SwitchServe()` is never automatic, because the rotation rules are not implemented.
- **Routes:** `GET /` (match list: create, select, finish), `GET /scoreboard/{id}/overlay` (OBS source), `GET /scoreboard/{id}/control`, `GET /api/games` (`?status=open` for open only), `GET /api/games/{id}`, `POST /api/games/{id}/finish`, `POST /api/games/{id}/replay`, `POST /api/games` (`{name, bestOf, teams}`), and per-game `GET /api/scoreboard/{id}/state` and `/events`, `POST /api/scoreboard/{id}/{point|games/add|games/remove|sets/add|sets/remove|serve|undo|teams|reset}`. Errors: `400` invalid id or JSON, `404` unknown game, `422` rule violation or invalid input, `503` OBS unavailable (replay), `500` database errors (logged, not shown).
- **Tests never touch the real database.** `internal/service` tests use `memoryStore` (`memory_store_test.go`), an in-memory fake of `GameStore`/`ScoreboardStore`. Do not add tests that connect to Postgres. SQL in `internal/repository` has no automated test; it was checked manually against the running database.

## Conventions

- Code identifiers, comments, and error messages are in English.
- Text shown to operators and on the broadcast (button labels, "Trocar saque", "Partida encerrada") stays in Portuguese.
- The module is named `broadcast`; the project folder is `transmissao`.
