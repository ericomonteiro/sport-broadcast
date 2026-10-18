// Command broadcast starts the scoreboard server to use with OBS.
//
// Usage:
//
//	go run ./cmd/broadcast -addr :8080 -best-of 3
//
// In OBS, add a "Browser" source with the URL http://localhost:8080/scoreboard/overlay.
// To operate the scoreboard, open http://localhost:8080/control in another tab or device.
package main

import (
	"flag"
	"log"
	"net/http"

	"broadcast/internal/scoreboard"
	"broadcast/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP address of the server")
	bestOf := flag.Int("best-of", 3, "number of sets in the match (1, 3 or 5)")
	flag.Parse()

	teams := [2]scoreboard.Team{
		{Players: []string{"Team A"}},
		{Players: []string{"Team B"}},
	}
	match, err := scoreboard.NewMatch(teams, *bestOf)
	if err != nil {
		log.Fatal(err)
	}

	srv := server.New(scoreboard.NewScoreboard(match))

	log.Printf("overlay: http://localhost%s/scoreboard/overlay", *addr)
	log.Printf("control: http://localhost%s/control", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.Handler()))
}
