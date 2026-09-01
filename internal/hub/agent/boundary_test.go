package agent

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAJustClaimedTaskIsNotABoundary pins what the deleted assignment window used to paper over: the
// moment a gate writes the claim, this reads mid-task. The gate no longer asks — it is inside the
// assignment it just made — so the read stays honest for everyone who does.
func TestAJustClaimedTaskIsNotABoundary(t *testing.T) {
	_, st := newService(t)
	s := New(st, clearTestDeps{}, nil)
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	// A claim just written, exactly as claimLeaf/startSubtask leaves it: Task set, Phase "working".
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: "td-abc123", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if at, err := s.AtLeafBoundary("proj", "eitri"); err != nil || at {
		t.Errorf("AtLeafBoundary = (%v, %v), want a held task read as mid-task", at, err)
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
	if err := st.For(api.GlobalProject).SetState(store.AgentState{Agent: "ori", Phase: "reviewing"}, store.ReasonClaimed, "test setup"); err != nil {
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
