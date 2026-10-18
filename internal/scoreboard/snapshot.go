package scoreboard

import "fmt"

// Snapshot is the storable form of a Match. Match keeps its fields private,
// so persistence goes through this struct. Undo history is not part of it.
type Snapshot struct {
	Teams    [2]Team  `json:"teams"`
	BestOf   int      `json:"bestOf"`
	Sets     [][2]int `json:"sets"`
	Games    [2]int   `json:"games"`
	Points   [2]int   `json:"points"`
	TieBreak bool     `json:"tieBreak"`
	Serving  int      `json:"serving"`
	Winner   int      `json:"winner"`
}

// Snapshot copies the match into its storable form.
func (m Match) Snapshot() Snapshot {
	sets := make([][2]int, m.numSets)
	copy(sets, m.sets[:m.numSets])

	return Snapshot{
		Teams:    m.Teams,
		BestOf:   m.BestOf,
		Sets:     sets,
		Games:    m.Games,
		Points:   m.Points,
		TieBreak: m.TieBreak,
		Serving:  m.Serving,
		Winner:   m.Winner,
	}
}

// RestoreMatch rebuilds a Match from a Snapshot, validating it first.
func RestoreMatch(s Snapshot) (Match, error) {
	m, err := NewMatch(s.Teams, s.BestOf)
	if err != nil {
		return Match{}, err
	}
	if len(s.Sets) > maxSets {
		return Match{}, fmt.Errorf("snapshot has %d sets, maximum is %d", len(s.Sets), maxSets)
	}
	if s.Serving != 0 && s.Serving != 1 {
		return Match{}, ErrInvalidTeam
	}
	if s.Winner < -1 || s.Winner > 1 {
		return Match{}, ErrInvalidTeam
	}

	m.numSets = copy(m.sets[:], s.Sets)
	m.Games = s.Games
	m.Points = s.Points
	m.TieBreak = s.TieBreak
	m.Serving = s.Serving
	m.Winner = s.Winner
	return m, nil
}
