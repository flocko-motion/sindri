package hub

import (
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// TestStalledForIsWhatTheBoardAndTheNudgeShare: the board word and the injected prod read one
// observation through this, so a user can never see "stalled" while nothing is told to the agent —
// or be told nothing while the board looks fine.
func TestStalledForIsWhatTheBoardAndTheNudgeShare(t *testing.T) {
	h := newHub(t)
	a := store.Agent{Project: "proj", Name: "dvalin"}

	// Working: not stalled, whatever the phase says.
	h.watch.record(a, true, 0, "working", false)
	if _, stalled := h.stalledFor("proj", "dvalin", "working"); stalled {
		t.Error("an agent that is working is not stalled")
	}

	// Idle, but the dwell has only just begun.
	h.watch.record(a, true, 0, "idle", false)
	idleFor, stalled := h.stalledFor("proj", "dvalin", "working")
	if stalled {
		t.Errorf("a fresh idle spell is a pause, not a stall (idle for %v)", idleFor)
	}

	// Backdate the spell past the dwell: now it is a stall.
	h.watch.mu.Lock()
	l := h.watch.obs[agentKey{"proj", "dvalin"}]
	l.idleSince = time.Now().Add(-workflow.StallDwell - time.Minute)
	h.watch.obs[agentKey{"proj", "dvalin"}] = l
	h.watch.mu.Unlock()

	idleFor, stalled = h.stalledFor("proj", "dvalin", "working")
	if !stalled {
		t.Errorf("a working agent idle for %v past the dwell should be stalled", idleFor)
	}
	if idleFor < workflow.StallDwell {
		t.Errorf("idleFor should report the whole spell, got %v", idleFor)
	}
	// The same observation, on a phase that exists to wait, is not a stall.
	if _, stalled := h.stalledFor("proj", "dvalin", "submitted"); stalled {
		t.Error("waiting on a verdict must never read as stalled")
	}
}

// TestStalledForNeedsAnObservation: an agent the watchdog has never seen, or one that is down, is
// not stalled — it is unknown or stopped, and both already show as themselves.
func TestStalledForNeedsAnObservation(t *testing.T) {
	h := newHub(t)
	if _, stalled := h.stalledFor("proj", "never-probed", "working"); stalled {
		t.Error("an unobserved agent must not read as stalled")
	}

	a := store.Agent{Project: "proj", Name: "gone"}
	for i := 0; i <= downStrikes; i++ {
		h.watch.record(a, false, 0, "", true) // conclusively down
	}
	if _, stalled := h.stalledFor("proj", "gone", "working"); stalled {
		t.Error("a down agent must not read as stalled")
	}
}
