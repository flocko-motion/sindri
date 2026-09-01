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
	a := store.Agent{Project: "proj", Name: "dvalin", Role: "worker"}
	ps := h.store.For("proj")
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	// The phase and the feature come off the STATE now, not the caller's arguments: the verdict is
	// the surface's, and it reads the same row every other rule does.
	holding := func(phase, container string) {
		t.Helper()
		if err := ps.SetState(store.AgentState{Agent: "dvalin", Phase: phase, Container: container},
			store.ReasonClaimed, "test setup"); err != nil {
			t.Fatal(err)
		}
	}

	// A screen that just changed: not stalled, whatever the phase says.
	holding("working", "")
	h.watch.record(a, true, 0, seen("working", "d1"))
	h.watch.record(a, true, 0, seen("working", "d2"))
	if _, stalled := h.stalledFor("proj", "dvalin"); stalled {
		t.Error("an agent whose pane is moving is not stalled")
	}

	// It has stopped moving, but the dwell has only just begun.
	h.watch.record(a, true, 0, seen("idle", "d2"))
	stillFor, stalled := h.stalledFor("proj", "dvalin")
	if stalled {
		t.Errorf("a fresh quiet spell is a pause, not a stall (still for %v)", stillFor)
	}

	// Backdate the spell past the dwell: now it is a stall.
	h.watch.mu.Lock()
	l := h.watch.obs[agentKey{"proj", "dvalin"}]
	l.stillSince = time.Now().Add(-workflow.StallDwell - time.Minute)
	h.watch.obs[agentKey{"proj", "dvalin"}] = l
	h.watch.mu.Unlock()

	stillFor, stalled = h.stalledFor("proj", "dvalin")
	if !stalled {
		t.Errorf("a worker whose screen stood still for %v past the dwell should be stalled", stillFor)
	}
	if stillFor < workflow.StallDwell {
		t.Errorf("stillFor should report the whole spell, got %v", stillFor)
	}
	// The same observation, on a phase that exists to wait, is not a stall.
	holding("submitted", "")
	if _, stalled := h.stalledFor("proj", "dvalin"); stalled {
		t.Error("waiting on a verdict must never read as stalled")
	}
	// A worker between subtasks of a feature it still holds has work to be getting on with — either
	// the next subtask or the submit — so sitting there IS a stall. This was excluded back when a
	// finished feature genuinely had to wait for a human to open its milestone PR.
	holding("idle", "td-EPIC")
	if _, stalled := h.stalledFor("proj", "dvalin"); !stalled {
		t.Error("a worker parked on a feature it holds should read as stalled")
	}
	// Holding nothing is idle, which is a state of its own and already shows as itself.
	holding("idle", "")
	if _, stalled := h.stalledFor("proj", "dvalin"); stalled {
		t.Error("an agent holding no work is idle, not stalled")
	}
}

// TestAStalledReviewerReadsAsStalledOnTheBoard is why the rule change is visible at all: stalledFor
// feeds both the board word and the nudge, so a reviewer that stopped mid-review now says so where
// the user looks, instead of sitting under the "reviewing" it was assigned hours ago.
func TestAStalledReviewerReadsAsStalledOnTheBoard(t *testing.T) {
	h := newHub(t)
	w := stillWatchdog(t, h)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: ".worktrees/ori"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "sd-1", Agent: "dvalin", Branch: "sd-1", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	id, err := ps.AddReview("pr-1", "review it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(id, "ori"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "ori", Phase: "reviewing"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	a := store.Agent{Project: testProject, Name: "ori"}
	w.record(a, true, 0, seen("idle", "d1"))
	standStill(t, w, testProject, "ori")

	view := onlyAgent(t, h)
	if view.Status != "stalled" {
		t.Errorf("a reviewer whose screen stood still past the dwell reads %q, want stalled", view.Status)
	}
	if view.PR != "pr-1" {
		t.Errorf("the row names PR %q, want the one it holds", view.PR)
	}
}

// standStill backdates an agent's quiet spell past the dwell, which is how a test reaches a stall
// without waiting minutes for one.
func standStill(t *testing.T, w *watchdog, project, name string) {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	l := w.obs[agentKey{project, name}]
	l.stillSince = time.Now().Add(-workflow.StallDwell - time.Minute)
	w.obs[agentKey{project, name}] = l
}

// TestStalledForLeavesAnAgentQueuedOnTheHubAlone: the pane signal (ToolRunning) cannot see this case
// at all — nothing is running in the agent's OWN pane, because it is waiting on the fleet's queue,
// the commonest instance of this same bug once gate results started caching.
func TestStalledForLeavesAnAgentQueuedOnTheHubAlone(t *testing.T) {
	h := newHub(t)
	a := store.Agent{Project: "proj", Name: "dvalin", Role: "worker"}
	if err := h.store.For("proj").PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if err := h.store.For("proj").SetState(store.AgentState{Agent: "dvalin", Phase: "working"},
		store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	h.watch.record(a, true, 0, seen("idle", "d1"))
	standStill(t, h.watch, "proj", "dvalin")

	if _, stalled := h.stalledFor("proj", "dvalin"); !stalled {
		t.Fatal("sanity: past the dwell with no run queued, this must already read as stalled")
	}

	ps := h.store.For("proj")
	if err := ps.PutRun(store.Run{ID: "run-1", Agent: "dvalin", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if _, stalled := h.stalledFor("proj", "dvalin"); stalled {
		t.Error("an agent waiting on its own queued run must not read as stalled")
	}
}

// TestStalledForNeedsAnObservation: an agent the watchdog has never seen, or one that is down, is
// not stalled — it is unknown or stopped, and both already show as themselves.
func TestStalledForNeedsAnObservation(t *testing.T) {
	h := newHub(t)
	if _, stalled := h.stalledFor("proj", "never-probed"); stalled {
		t.Error("an unobserved agent must not read as stalled")
	}

	a := store.Agent{Project: "proj", Name: "gone"}
	for i := 0; i <= downStrikes; i++ {
		h.watch.record(a, false, 0, agent.Observation{}) // past the strike threshold: down
	}
	if _, stalled := h.stalledFor("proj", "gone"); stalled {
		t.Error("a down agent must not read as stalled")
	}
}
