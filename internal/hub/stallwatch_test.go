package hub

import (
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// TestStalledForIsWhatTheBoardAndTheNudgeShare: the board word and the injected prod read one
// observation through this, so a user can never see "stalled" while nothing is told to the agent —
// or be told nothing while the board looks fine.
func TestStalledForIsWhatTheBoardAndTheNudgeShare(t *testing.T) {
	h := newHub(t)
	a := store.Agent{Project: "proj", Name: "dvalin"}

	// A screen that just changed: not stalled, whatever the phase says.
	h.watch.record(a, true, 0, seen("working", "d1"))
	h.watch.record(a, true, 0, seen("working", "d2"))
	if _, stalled := h.stalledFor("proj", "dvalin", "working", ""); stalled {
		t.Error("an agent whose pane is moving is not stalled")
	}

	// It has stopped moving, but the dwell has only just begun.
	h.watch.record(a, true, 0, seen("idle", "d2"))
	stillFor, stalled := h.stalledFor("proj", "dvalin", "working", "")
	if stalled {
		t.Errorf("a fresh quiet spell is a pause, not a stall (still for %v)", stillFor)
	}

	// Backdate the spell past the dwell: now it is a stall.
	h.watch.mu.Lock()
	l := h.watch.obs[agentKey{"proj", "dvalin"}]
	l.stillSince = time.Now().Add(-workflow.StallDwell - time.Minute)
	h.watch.obs[agentKey{"proj", "dvalin"}] = l
	h.watch.mu.Unlock()

	stillFor, stalled = h.stalledFor("proj", "dvalin", "working", "")
	if !stalled {
		t.Errorf("a worker whose screen stood still for %v past the dwell should be stalled", stillFor)
	}
	if stillFor < workflow.StallDwell {
		t.Errorf("stillFor should report the whole spell, got %v", stillFor)
	}
	// The same observation, on a phase that exists to wait, is not a stall.
	if _, stalled := h.stalledFor("proj", "dvalin", "submitted", ""); stalled {
		t.Error("waiting on a verdict must never read as stalled")
	}
	// A worker between subtasks of a feature it still holds has work to be getting on with — either
	// the next subtask or the submit — so sitting there IS a stall. This was excluded back when a
	// finished feature genuinely had to wait for a human to open its milestone PR.
	if _, stalled := h.stalledFor("proj", "dvalin", "idle", "td-EPIC"); !stalled {
		t.Error("a worker parked on a feature it holds should read as stalled")
	}
	// Holding nothing is idle, which is a state of its own and already shows as itself.
	if _, stalled := h.stalledFor("proj", "dvalin", "idle", ""); stalled {
		t.Error("an agent holding no work is idle, not stalled")
	}
}

// TestStalledForNeedsAnObservation: an agent the watchdog has never seen, or one that is down, is
// not stalled — it is unknown or stopped, and both already show as themselves.
func TestStalledForNeedsAnObservation(t *testing.T) {
	h := newHub(t)
	if _, stalled := h.stalledFor("proj", "never-probed", "working", ""); stalled {
		t.Error("an unobserved agent must not read as stalled")
	}

	a := store.Agent{Project: "proj", Name: "gone"}
	for i := 0; i <= downStrikes; i++ {
		h.watch.record(a, false, 0, agent.Observation{}) // past the strike threshold: down
	}
	if _, stalled := h.stalledFor("proj", "gone", "working", ""); stalled {
		t.Error("a down agent must not read as stalled")
	}
}
