package obs

import (
	"testing"

	"github.com/andreykaipov/goobs/api/events"
)

func TestSavedReplayUsesRequestedLength(t *testing.T) {
	c := New("localhost:4455", "", "Replay", "Replay Clip", "Replay Intro", "Live")
	c.pending = []int{10, 15}

	c.onEvent(&events.ReplayBufferSaved{SavedReplayPath: "/videos/a.mov"})
	c.onEvent(&events.ReplayBufferSaved{SavedReplayPath: "/videos/b.mov"})

	first := <-c.saved
	second := <-c.saved
	if first != (clip{path: "/videos/a.mov", seconds: 10}) {
		t.Fatalf("first clip = %+v", first)
	}
	if second != (clip{path: "/videos/b.mov", seconds: 15}) {
		t.Fatalf("second clip = %+v", second)
	}
}

func TestSavedReplayWithoutRequestPlaysInFull(t *testing.T) {
	c := New("localhost:4455", "", "Replay", "Replay Clip", "Replay Intro", "Live")

	c.onEvent(&events.ReplayBufferSaved{SavedReplayPath: "/videos/hotkey.mov"})

	if got := <-c.saved; got.seconds != fullReplaySeconds {
		t.Fatalf("seconds = %d, want %d", got.seconds, fullReplaySeconds)
	}
}

func TestSeekOffset(t *testing.T) {
	cases := []struct {
		durationMs float64
		seconds    int
		want       float64
	}{
		{20000, 5, 15000},  // 5s replay of a 20s clip starts at 15s
		{20000, 10, 10000}, // 10s replay of a 20s clip starts at 10s
		{20000, 15, 5000},  // 15s replay starts at 5s
		{20000, 20, 0},     // full replay starts at the beginning
		{12000, 15, 0},     // clip shorter than requested plays in full
	}
	for _, tc := range cases {
		if got := seekOffset(tc.durationMs, tc.seconds); got != tc.want {
			t.Errorf("seekOffset(%v, %d) = %v, want %v", tc.durationMs, tc.seconds, got, tc.want)
		}
	}
}
