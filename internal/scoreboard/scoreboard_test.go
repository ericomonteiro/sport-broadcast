package scoreboard

import (
	"fmt"
	"testing"
)

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

func TestAddGameClosesSet(t *testing.T) {
	s := newTestScoreboard(t, 3)
	for i := 0; i < 5; i++ {
		s.AddGame(0)
	}
	if v := s.State().View(); v.Games != [2]int{5, 0} || len(v.Sets) != 0 {
		t.Fatalf("expected games 5-0 with no set, got games=%v sets=%v", v.Games, v.Sets)
	}

	s.AddGame(0)
	if v := s.State().View(); len(v.Sets) != 1 || v.Sets[0] != [2]int{6, 0} {
		t.Fatalf("expected set 6-0 after sixth game, got %v", v.Sets)
	}
}

func TestAddGameDuringTieBreakIsRejected(t *testing.T) {
	s := newTestScoreboard(t, 3)
	for i := 0; i < 6; i++ {
		winGame(t, s, 0)
		winGame(t, s, 1)
	}
	if err := s.AddGame(0); err != ErrTieBreakInProgress {
		t.Fatalf("expected ErrTieBreakInProgress, got %v", err)
	}
}

func TestRemoveGame(t *testing.T) {
	s := newTestScoreboard(t, 3)
	if err := s.RemoveGame(0); err != ErrNothingToRemove {
		t.Fatalf("expected ErrNothingToRemove on 0 games, got %v", err)
	}

	s.AddGame(0)
	s.AddGame(0)
	s.RemoveGame(0)
	if v := s.State().View(); v.Games != [2]int{1, 0} {
		t.Fatalf("expected games 1-0, got %v", v.Games)
	}
}

func TestAddSetUsesFinalScore(t *testing.T) {
	s := newTestScoreboard(t, 3)
	s.AddGame(1)
	s.AddGame(1)
	s.AddSet(0)
	if v := s.State().View(); len(v.Sets) != 1 || v.Sets[0] != [2]int{6, 2} || v.SetsWon != [2]int{1, 0} {
		t.Fatalf("expected set 6-2 for team 0, got sets=%v setsWon=%v", v.Sets, v.SetsWon)
	}

	// Tie-break case: 6-6 then the team wins the set -> 7-6.
	s = newTestScoreboard(t, 3)
	for i := 0; i < 6; i++ {
		winGame(t, s, 0)
		winGame(t, s, 1)
	}
	s.AddSet(1)
	if v := s.State().View(); len(v.Sets) != 1 || v.Sets[0] != [2]int{6, 7} || v.TieBreak {
		t.Fatalf("expected set 6-7 with tie-break closed, got sets=%v tieBreak=%v", v.Sets, v.TieBreak)
	}
}

func TestAddSetCanFinishMatch(t *testing.T) {
	s := newTestScoreboard(t, 1)
	s.AddSet(1)
	if v := s.State().View(); v.Winner != 1 {
		t.Fatalf("expected team 1 to win a best-of-1 match, got winner %d", v.Winner)
	}
}

func TestRemoveSetReopensMatch(t *testing.T) {
	s := newTestScoreboard(t, 1)
	s.AddSet(0)
	if err := s.RemoveSet(1); err != ErrNothingToRemove {
		t.Fatalf("expected ErrNothingToRemove for team without sets, got %v", err)
	}
	if err := s.RemoveSet(0); err != nil {
		t.Fatalf("RemoveSet: %v", err)
	}
	if v := s.State().View(); len(v.Sets) != 0 || v.Winner != -1 {
		t.Fatalf("expected match reopened with no sets, got %+v", v)
	}
}

func TestUndoRevertsCorrections(t *testing.T) {
	s := newTestScoreboard(t, 3)
	s.AddSet(0)
	s.Undo()
	if v := s.State().View(); len(v.Sets) != 0 || v.SetsWon != [2]int{0, 0} {
		t.Fatalf("expected undo to revert AddSet, got %+v", v)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	s := newTestScoreboard(t, 3)
	s.AddSet(0)
	s.AddGame(1)
	s.Point(1)
	s.SwitchServe()

	restored, err := RestoreMatch(s.State().Snapshot())
	if err != nil {
		t.Fatalf("RestoreMatch: %v", err)
	}
	if got, want := restored.View(), s.State().View(); !equalViews(got, want) {
		t.Fatalf("round trip changed the match:\n got  %+v\n want %+v", got, want)
	}
}

func TestRestoreMatchRejectsInvalidSnapshot(t *testing.T) {
	base := newTestScoreboard(t, 3).State().Snapshot()

	tooManySets := base
	tooManySets.Sets = make([][2]int, maxSets+1)
	if _, err := RestoreMatch(tooManySets); err == nil {
		t.Fatalf("expected error for too many sets")
	}

	badServing := base
	badServing.Serving = 2
	if _, err := RestoreMatch(badServing); err != ErrInvalidTeam {
		t.Fatalf("expected ErrInvalidTeam for serving=2, got %v", err)
	}

	badBestOf := base
	badBestOf.BestOf = 4
	if _, err := RestoreMatch(badBestOf); err != ErrInvalidBestOf {
		t.Fatalf("expected ErrInvalidBestOf, got %v", err)
	}
}

func equalViews(a, b View) bool {
	if len(a.Sets) != len(b.Sets) {
		return false
	}
	for i := range a.Sets {
		if a.Sets[i] != b.Sets[i] {
			return false
		}
	}
	a.Sets, b.Sets = nil, nil
	return fmt.Sprintf("%+v", a) == fmt.Sprintf("%+v", b)
}
