package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/harness"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestALookForAnActionSettlesAtOnce is the start of a dead coauthor: its pod had exited, the sweep
// held it "up" for downStrikes, and a human's start was answered from that — the machine left its
// launching state on the held "up" and launched nothing. A look taken for a caller about to act
// settles on its own reading, so what that caller acts on is what the machine reads.
func TestALookForAnActionSettlesAtOnce(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "nyi"}

	w.record(a, true, 1, seen("idle", "d1"))
	w.recordReading(a, false, 0, harness.Observation{}, true)
	if l, _ := w.get("proj", "nyi"); l.up {
		t.Error("a settling reading of a gone pod must report down at once, not after downStrikes")
	}

	// And a sweep's failure after it finds nothing to hold: the agent stays down.
	w.record(a, false, 0, harness.Observation{})
	if l, _ := w.get("proj", "nyi"); l.up {
		t.Error("a sweep after the settling reading brought the dead pod back up")
	}
}
