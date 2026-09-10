package agent

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// awaitingAuthor seeds a worker holding no task in its state row, with one PR of the given status
// against sd-1 — the shape a submit leaves, and the shape a rejection returns to.
func awaitingAuthor(t *testing.T, status string) *Service {
	t.Helper()
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-sd-1", Task: "sd-1", Agent: "eitri", Branch: "sd-1", Status: status}); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestAPRAwaitingAVerdictIsNotABoundary: austri sat waiting on a review with its state row empty,
// so every reader called it free. Compaction there cuts away the context that knows what it awaits.
func TestAPRAwaitingAVerdictIsNotABoundary(t *testing.T) {
	for _, status := range []string{"open", "rejected", "approved"} {
		s := awaitingAuthor(t, status)
		if at, err := s.AtLeafBoundary("proj", "eitri"); err != nil || at {
			t.Errorf("status %q: AtLeafBoundary = (%v, %v), want held — the task is its until the PR lands", status, at, err)
		}
		if nothing, err := s.HoldsNothing("proj", "eitri", "worker"); err != nil || nothing {
			t.Errorf("status %q: HoldsNothing = (%v, %v), want holding", status, nothing, err)
		}
	}
}

// TestAClosedTaskEndsTheHold: "rejected" is not terminal, since a resubmission clears it — so a
// rejected PR over a CLOSED task is held work with nothing left to fix and no way to discharge it.
// austri carried pr-sd-a47b61 that way for four days, idle and unable to take anything else.
func TestAClosedTaskEndsTheHold(t *testing.T) {
	s := awaitingAuthor(t, "rejected")
	ps := s.store.For("proj")
	if err := ps.UpsertTask(store.Task{ID: "sd-1", Status: "closed"}); err != nil {
		t.Fatal(err)
	}
	if nothing, err := s.HoldsNothing("proj", "eitri", "worker"); err != nil || !nothing {
		t.Errorf("HoldsNothing = (%v, %v); a closed task leaves its PR nothing to land into", nothing, err)
	}
	if at, err := s.AtLeafBoundary("proj", "eitri"); err != nil || !at {
		t.Errorf("AtLeafBoundary = (%v, %v), want a boundary once the task is closed", at, err)
	}
}

// TestASettledPRHoldsNothing is the control: once it merges or is scrapped the work really is over,
// and an agent that can never be freed is as broken as one freed too early.
func TestASettledPRHoldsNothing(t *testing.T) {
	for _, status := range []string{"merged", "scrapped"} {
		s := awaitingAuthor(t, status)
		if at, err := s.AtLeafBoundary("proj", "eitri"); err != nil || !at {
			t.Errorf("status %q: AtLeafBoundary = (%v, %v), want a boundary", status, at, err)
		}
		if nothing, err := s.HoldsNothing("proj", "eitri", "worker"); err != nil || !nothing {
			t.Errorf("status %q: HoldsNothing = (%v, %v), want nothing held", status, nothing, err)
		}
	}
}

// TestATaskGivenToSomebodyElseEndsTheHold: the TASK decides what an agent holds, and a PR against
// one it no longer has is nobody's business. sudri's feature was released to dvalin, yet its PR kept
// pointing back — so `sindri task` said "you hold no task" while the directive sent it to submit.
func TestATaskGivenToSomebodyElseEndsTheHold(t *testing.T) {
	s := awaitingAuthor(t, "rejected")
	ps := s.store.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	// The task moves on, exactly as a release leaves it.
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "sd-1", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}

	if nothing, err := s.HoldsNothing("proj", "eitri", "worker"); err != nil || !nothing {
		t.Errorf("HoldsNothing = (%v, %v); the work is dvalin's now", nothing, err)
	}
	if at, err := s.AtLeafBoundary("proj", "eitri"); err != nil || !at {
		t.Errorf("AtLeafBoundary = (%v, %v), want a boundary once the task is gone", at, err)
	}
}
