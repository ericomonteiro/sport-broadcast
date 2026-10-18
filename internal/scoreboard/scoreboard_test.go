package scoreboard

import "testing"

func newTestScoreboard(t *testing.T, bestOf int) *Scoreboard {
	t.Helper()
	m, err := NewMatch([2]Team{
		{Players: []string{"Erico", "João"}},
		{Players: []string{"Pedro", "Lucas"}},
	}, bestOf)
	if err != nil {
		t.Fatalf("NewMatch: %v", err)
	}
	return NewScoreboard(m)
}

// winGame makes a team win a game without the opponent scoring.
func winGame(t *testing.T, s *Scoreboard, team int) {
	t.Helper()
	for i := 0; i < 4; i++ {
		if err := s.Point(team); err != nil {
			t.Fatalf("Point: %v", err)
		}
	}
}

func TestGameScoring(t *testing.T) {
	s := newTestScoreboard(t, 3)

	s.Point(0)
	s.Point(0)
	if v := s.State().View(); v.Points != [2]string{"30", "0"} {
		t.Fatalf("expected 30-0, got %v", v.Points)
	}

	s.Point(0)
	s.Point(1)
	s.Point(1)
	s.Point(1)
	if v := s.State().View(); v.Points != [2]string{"40", "40"} {
		t.Fatalf("expected 40-40, got %v", v.Points)
	}

	s.Point(0)
	if v := s.State().View(); v.Points != [2]string{"Vant.", "40"} {
		t.Fatalf("expected advantage for team 0, got %v", v.Points)
	}

	s.Point(1)
	if v := s.State().View(); v.Points != [2]string{"40", "40"} {
		t.Fatalf("expected back to 40-40, got %v", v.Points)
	}

	s.Point(1)
	s.Point(1)
	if v := s.State().View(); v.Games != [2]int{0, 1} || v.Points != [2]string{"0", "0"} {
		t.Fatalf("expected game for team 1, got games=%v points=%v", v.Games, v.Points)
	}
}

func TestSetWonSixZero(t *testing.T) {
	s := newTestScoreboard(t, 3)
	for i := 0; i < 6; i++ {
		winGame(t, s, 0)
	}

	v := s.State().View()
	if len(v.Sets) != 1 || v.Sets[0] != [2]int{6, 0} {
		t.Fatalf("expected set 6-0, got %v", v.Sets)
	}
	if v.SetsWon != [2]int{1, 0} || v.Winner != -1 {
		t.Fatalf("expected 1 set for team 0 and match in progress, got %+v", v)
	}
}

func TestSetNotWonAtSixFive(t *testing.T) {
	s := newTestScoreboard(t, 3)
	for i := 0; i < 5; i++ {
		winGame(t, s, 0)
		winGame(t, s, 1)
	}
	// Games are 5-5. Team 0 wins a game -> 6-5, the set continues.
	winGame(t, s, 0)
	if v := s.State().View(); len(v.Sets) != 0 || v.Games != [2]int{6, 5} {
		t.Fatalf("expected games 6-5 with no finished set, got games=%v sets=%v", v.Games, v.Sets)
	}
}

func TestTieBreakAndSetWinner(t *testing.T) {
	s := newTestScoreboard(t, 3)
	// Alternate games until 6-6.
	for i := 0; i < 6; i++ {
		winGame(t, s, 0)
		winGame(t, s, 1)
	}
	if v := s.State().View(); !v.TieBreak {
		t.Fatalf("expected tie-break at 6-6")
	}

	for i := 0; i < 7; i++ {
		s.Point(0)
	}

	v := s.State().View()
	if len(v.Sets) != 1 || v.Sets[0] != [2]int{7, 6} {
		t.Fatalf("expected set 7-6, got %v", v.Sets)
	}
	if v.TieBreak {
		t.Fatalf("tie-break should have ended")
	}
}

func TestMatchEndsAndRejectsPoints(t *testing.T) {
	s := newTestScoreboard(t, 3)
	for set := 0; set < 2; set++ {
		for i := 0; i < 6; i++ {
			winGame(t, s, 1)
		}
	}

	v := s.State().View()
	if v.Winner != 1 {
		t.Fatalf("expected team 1 as winner, got %d", v.Winner)
	}
	if err := s.Point(0); err != ErrMatchOver {
		t.Fatalf("expected ErrMatchOver, got %v", err)
	}
}

func TestUndo(t *testing.T) {
	s := newTestScoreboard(t, 3)
	s.Point(0)
	s.Point(1)

	if !s.Undo() {
		t.Fatalf("expected something to undo")
	}
	if v := s.State().View(); v.Points != [2]string{"15", "0"} {
		t.Fatalf("expected 15-0 after undo, got %v", v.Points)
	}

	s.Undo()
	if s.Undo() {
		t.Fatalf("there should be nothing left to undo")
	}
}

func TestUndoKeepsTeamNames(t *testing.T) {
	s := newTestScoreboard(t, 3)
	s.Point(0)
	s.SetTeams([2]Team{{Players: []string{"New A"}}, {Players: []string{"New B"}}})
	s.Undo()

	if name := s.State().Teams[0].Name(); name != "New A" {
		t.Fatalf("undo should keep the current team names, got %q", name)
	}
}

func TestInvalidBestOf(t *testing.T) {
	if _, err := NewMatch([2]Team{}, 2); err != ErrInvalidBestOf {
		t.Fatalf("expected ErrInvalidBestOf, got %v", err)
	}
}

func TestResetKeepsTeamNames(t *testing.T) {
	s := newTestScoreboard(t, 3)
	s.Point(0)
	if err := s.Reset(1); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	v := s.State().View()
	if v.Teams[0].Name() != "Erico / João" || v.Points != [2]string{"0", "0"} || v.BestOf != 1 {
		t.Fatalf("unexpected reset: %+v", v)
	}
}
