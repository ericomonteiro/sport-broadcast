// Package scoreboard holds the scoring rules for a beach tennis match.
//
// Rules (change here if the tournament uses a different format):
//   - Game: points 0, 15, 30, 40 and advantage; a game is won with 4 points and a 2-point lead.
//   - Set: won with 6 games and a 2-game lead; at 6-6 a tie-break is played up to 7 points.
//   - Match: best of 1, 3 or 5 sets.
package scoreboard

import (
	"errors"
	"strconv"
	"strings"
)

const maxSets = 5

var (
	ErrMatchOver          = errors.New("match is already over")
	ErrInvalidTeam        = errors.New("invalid team")
	ErrInvalidBestOf      = errors.New("bestOf must be 1, 3 or 5")
	ErrNothingToRemove    = errors.New("nothing to remove")
	ErrTieBreakInProgress = errors.New("cannot change games during a tie-break")
)

// Team is a pair of players (or a single player in singles).
type Team struct {
	Players []string `json:"players"`
}

// Name joins the players, e.g. "Erico / João".
func (t Team) Name() string {
	return strings.Join(t.Players, " / ")
}

// Match is the full state of a game. It only uses fixed-size arrays so the
// struct can be copied by value (this is how undo works).
type Match struct {
	Teams    [2]Team
	BestOf   int
	sets     [maxSets][2]int // games of each finished set
	numSets  int
	Games    [2]int // games in the current set
	Points   [2]int // points in the current game (or in the tie-break)
	TieBreak bool
	Serving  int // index of the team serving
	Winner   int // -1 while the match is in progress
}

// NewMatch creates a match with no points played.
func NewMatch(teams [2]Team, bestOf int) (Match, error) {
	if bestOf != 1 && bestOf != 3 && bestOf != 5 {
		return Match{}, ErrInvalidBestOf
	}
	return Match{Teams: teams, BestOf: bestOf, Winner: -1}, nil
}

// Point awards a point to the given team (0 or 1).
func (m *Match) Point(team int) error {
	if err := m.checkPlayable(team); err != nil {
		return err
	}

	other := 1 - team
	m.Points[team]++

	if m.TieBreak {
		if m.Points[team] >= 7 && m.Points[team]-m.Points[other] >= 2 {
			m.winSet(team)
		}
		return nil
	}

	if m.Points[team] >= 4 && m.Points[team]-m.Points[other] >= 2 {
		m.winGame(team)
	}
	return nil
}

// AddGame gives the team a game, as if it had been won on the court.
// It follows the same rules as a game won by points, so it can close a set.
func (m *Match) AddGame(team int) error {
	if err := m.checkPlayable(team); err != nil {
		return err
	}
	if m.TieBreak {
		return ErrTieBreakInProgress
	}
	m.winGame(team)
	return nil
}

// RemoveGame takes one game away from the team in the current set (correction).
func (m *Match) RemoveGame(team int) error {
	if err := m.checkPlayable(team); err != nil {
		return err
	}
	if m.TieBreak {
		return ErrTieBreakInProgress
	}
	if m.Games[team] == 0 {
		return ErrNothingToRemove
	}
	m.Games[team]--
	m.Points = [2]int{}
	return nil
}

// AddSet gives the team the current set, with the usual final score
// (6-x, 7-5 or 7-6). The current games are replaced by that score.
func (m *Match) AddSet(team int) error {
	if err := m.checkPlayable(team); err != nil {
		return err
	}
	other := 1 - team
	m.TieBreak = false
	if m.Games[other] >= 6 {
		m.Games[team] = 7
	} else {
		m.Games[team] = max(6, m.Games[other]+2)
	}
	m.winSet(team)
	return nil
}

// RemoveSet removes the most recent set won by the team (correction).
// The match can be reopened this way.
func (m *Match) RemoveSet(team int) error {
	if team != 0 && team != 1 {
		return ErrInvalidTeam
	}
	for i := m.numSets - 1; i >= 0; i-- {
		if setWinner(m.sets[i]) == team {
			copy(m.sets[i:], m.sets[i+1:m.numSets])
			m.numSets--
			m.recomputeWinner()
			return nil
		}
	}
	return ErrNothingToRemove
}

// SwitchServe passes the serve to the other team. It is manual on purpose,
// because the operator knows best when the serve changes.
func (m *Match) SwitchServe() {
	m.Serving = 1 - m.Serving
}

// SetsWon counts how many sets each team has won.
func (m Match) SetsWon() [2]int {
	var won [2]int
	for i := 0; i < m.numSets; i++ {
		won[setWinner(m.sets[i])]++
	}
	return won
}

func (m Match) checkPlayable(team int) error {
	if m.Winner >= 0 {
		return ErrMatchOver
	}
	if team != 0 && team != 1 {
		return ErrInvalidTeam
	}
	return nil
}

func setWinner(set [2]int) int {
	if set[0] > set[1] {
		return 0
	}
	return 1
}

func (m *Match) recomputeWinner() {
	won := m.SetsWon()
	m.Winner = -1
	for team := range won {
		if won[team] > m.BestOf/2 {
			m.Winner = team
		}
	}
}

func (m *Match) winGame(team int) {
	other := 1 - team
	m.Games[team]++
	m.Points = [2]int{}

	if m.Games[team] >= 6 && m.Games[team]-m.Games[other] >= 2 {
		m.winSet(team)
		return
	}
	if m.Games[0] == 6 && m.Games[1] == 6 {
		m.TieBreak = true
	}
}

func (m *Match) winSet(team int) {
	if m.TieBreak {
		// In a tie-break the winner goes from 6 to 7 games.
		m.Games[team]++
	}
	m.sets[m.numSets] = m.Games
	m.numSets++

	m.Games = [2]int{}
	m.Points = [2]int{}
	m.TieBreak = false

	m.recomputeWinner()
}

// View is the match in display format, used by the overlay and the control panel (JSON).
type View struct {
	Teams    [2]Team   `json:"teams"`
	BestOf   int       `json:"bestOf"`
	Sets     [][2]int  `json:"sets"`
	SetsWon  [2]int    `json:"setsWon"`
	Games    [2]int    `json:"games"`
	Points   [2]string `json:"points"`
	TieBreak bool      `json:"tieBreak"`
	Serving  int       `json:"serving"`
	Winner   int       `json:"winner"`
}

// View converts the match into display values (e.g. "15", "40", "Vant.").
func (m Match) View() View {
	sets := make([][2]int, m.numSets)
	copy(sets, m.sets[:m.numSets])

	return View{
		Teams:   m.Teams,
		BestOf:  m.BestOf,
		Sets:    sets,
		SetsWon: m.SetsWon(),
		Games:   m.Games,
		Points: [2]string{
			pointLabel(m.Points[0], m.Points[1], m.TieBreak),
			pointLabel(m.Points[1], m.Points[0], m.TieBreak),
		},
		TieBreak: m.TieBreak,
		Serving:  m.Serving,
		Winner:   m.Winner,
	}
}

var gamePointLabels = [...]string{"0", "15", "30", "40"}

func pointLabel(points, opponent int, tieBreak bool) string {
	if tieBreak {
		return strconv.Itoa(points)
	}
	if points >= 3 && opponent >= 3 {
		if points > opponent {
			return "Vant."
		}
		return "40"
	}
	if points > 3 {
		points = 3
	}
	return gamePointLabels[points]
}

// Scoreboard wraps the current match and its history so points can be undone.
type Scoreboard struct {
	current Match
	history []Match
}

// NewScoreboard creates a scoreboard for the given match.
func NewScoreboard(m Match) *Scoreboard {
	return &Scoreboard{current: m}
}

// Point records a point and saves the previous state for undo.
func (s *Scoreboard) Point(team int) error {
	return s.apply(func(m *Match) error { return m.Point(team) })
}

// AddGame, RemoveGame, AddSet and RemoveSet are manual corrections. They are
// saved in the history too, so Undo reverts them.
func (s *Scoreboard) AddGame(team int) error {
	return s.apply(func(m *Match) error { return m.AddGame(team) })
}

func (s *Scoreboard) RemoveGame(team int) error {
	return s.apply(func(m *Match) error { return m.RemoveGame(team) })
}

func (s *Scoreboard) AddSet(team int) error {
	return s.apply(func(m *Match) error { return m.AddSet(team) })
}

func (s *Scoreboard) RemoveSet(team int) error {
	return s.apply(func(m *Match) error { return m.RemoveSet(team) })
}

// apply runs the change on a copy and only commits it (and saves the
// previous state) if it succeeds.
func (s *Scoreboard) apply(change func(*Match) error) error {
	next := s.current
	if err := change(&next); err != nil {
		return err
	}
	s.history = append(s.history, s.current)
	s.current = next
	return nil
}

// Undo returns to the state before the last point. Returns false if there is nothing to undo.
func (s *Scoreboard) Undo() bool {
	if len(s.history) == 0 {
		return false
	}
	// Team names are not part of the history, so we keep the current ones.
	teams := s.current.Teams
	s.current = s.history[len(s.history)-1]
	s.current.Teams = teams
	s.history = s.history[:len(s.history)-1]
	return true
}

// State returns a copy of the current match.
func (s *Scoreboard) State() Match {
	return s.current
}

// SwitchServe toggles the serve.
func (s *Scoreboard) SwitchServe() {
	s.current.SwitchServe()
}

// SetTeams changes the team names without resetting the score.
func (s *Scoreboard) SetTeams(teams [2]Team) {
	s.current.Teams = teams
}

// Reset clears the score and keeps the team names.
func (s *Scoreboard) Reset(bestOf int) error {
	next, err := NewMatch(s.current.Teams, bestOf)
	if err != nil {
		return err
	}
	s.current = next
	s.history = nil
	return nil
}
