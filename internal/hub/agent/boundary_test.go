package agent

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestBeginAssignmentAdmitsAFreshClaim is the trap a claim-first assignment gate falls into
// otherwise: the moment the gate writes the claim (agent_state.task), AtLeafBoundary reads it as
// mid-task and refuses the very preparation the gate is about to run. BeginAssignment is the gate's
// own signal that nothing has happened under this claim yet, so the boundary still holds.
func TestBeginAssignmentAdmitsAFreshClaim(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	// A claim just written, exactly as claimLeaf/startSubtask leaves it: Task set, Phase "working".
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: "td-abc123", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if at, _ := s.AtLeafBoundary("proj", "eitri"); at {
		t.Fatal("without BeginAssignment this is an ordinary held task — not a boundary")
	}

	s.BeginAssignment("proj", "eitri")
	if at, err := s.AtLeafBoundary("proj", "eitri"); err != nil || !at {
		t.Errorf("AtLeafBoundary = (%v, %v), want the just-claimed task admitted as a boundary", at, err)
	}

	s.EndAssignment("proj", "eitri")
	if at, _ := s.AtLeafBoundary("proj", "eitri"); at {
		t.Error("once preparation ends, the same held task must read as mid-task again")
	}
}

// TestBeginAssignmentIsPerAgent: one agent's in-flight preparation must not paper over a genuinely
// mid-task reading for another.
func TestBeginAssignmentIsPerAgent(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	for _, name := range []string{"eitri", "nori"} {
		if err := ps.PutAgent(store.Agent{Name: name, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
		if err := ps.SetState(store.AgentState{Agent: name, Task: "td-1", Phase: "working"}); err != nil {
			t.Fatal(err)
		}
	}
	s.BeginAssignment("proj", "eitri")
	defer s.EndAssignment("proj", "eitri")

	if at, _ := s.AtLeafBoundary("proj", "nori"); at {
		t.Error("nori's own mid-task hold must not be admitted by eitri's in-flight preparation")
	}
}

// TestAPooledReviewerMidReviewIsNotABoundary is the safety-critical gap a review found: a
// GlobalProject reviewer's held review is filed under the PR's own project, never GlobalProject
// itself, so a project-scoped read used to say "boundary" while it was reading a diff — the exact
// moment /clear or compaction must not fire, since it would silently invalidate that reading.
func TestAPooledReviewerMidReviewIsNotABoundary(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	if err := st.For(api.GlobalProject).PutAgent(store.Agent{Name: "ori", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	if err := st.For(api.GlobalProject).SetState(store.AgentState{Agent: "ori", Phase: "reviewing"}); err != nil {
		t.Fatal(err)
	}
	repo := st.For("repo")
	if err := repo.PutPR(store.PR{ID: "pr-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	rid, err := repo.AddReview("pr-1", "look")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AssignReview(rid, "ori"); err != nil {
		t.Fatal(err)
	}

	if at, err := s.AtLeafBoundary(api.GlobalProject, "ori"); err != nil || at {
		t.Errorf("AtLeafBoundary = (%v, %v), want false — ori holds pr-1 in repo, not nothing", at, err)
	}
}
