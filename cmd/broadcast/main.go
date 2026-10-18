// Command broadcast starts the scoreboard server. It needs a running Postgres
// (see docker-compose.yml) and the settings in application-<ENVIRONMENT>.properties
// or in environment variables (see internal/config).
//
// Usage:
//
//	ENVIRONMENT=local go run ./cmd/broadcast
//
// Open http://localhost:8080/ to create matches. In OBS, add a "Browser" source
// with http://localhost:8080/scoreboard/<game id>/overlay.
package main

import (
	"context"
	"log"
	"net/http"

	"broadcast/internal/config"
	"broadcast/internal/handler"
	"broadcast/internal/infra/database"
	"broadcast/internal/repository"
	"broadcast/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL())
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	games := repository.NewGames(pool)
	boards := repository.NewScoreboards(pool)
	h := handler.New(
		service.NewGameService(games),
		service.NewScoreboardService(games, boards),
	)

	log.Printf("environment: %q, listening on %s", cfg.Environment, cfg.ServerAddr)
	log.Fatal(http.ListenAndServe(cfg.ServerAddr, h.Routes()))
}
