package obs

import (
	"testing"

	"github.com/andreykaipov/goobs/api/events"
)

func TestSavedReplayIsQueued(t *testing.T) {
	c := New("localhost:4455", "", "Replay", "Replay Clip", "Replay Intro")

	c.onEvent(&events.ReplayBufferSaved{SavedReplayPath: "/videos/Replay 1.mkv"})

	select {
	case path := <-c.saved:
		if path != "/videos/Replay 1.mkv" {
			t.Fatalf("queued path = %q", path)
		}
	default:
		t.Fatalf("expected the replay to be queued")
	}
}
