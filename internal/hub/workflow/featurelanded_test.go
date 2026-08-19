package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAnInterimMergeDoesNotLandTheFeature is the bug `sindri contribute` hit in practice: its PR
// merges with Kind "interim" — a milestone, not the feature's own end — so featureLanded must not
// read that merge as the whole feature landing. Getting this wrong strands a worker mid-feature:
// AgentDirective clears its Container, and the next ask re-offers the feature as a fresh claim,
// which finds its actually-unfinished subtask not "open" (it's in_progress, still held) and
// declares the feature done.
func TestAnInterimMergeDoesNotLandTheFeature(t *testing.T) {
	e, ps, _, _ := featureWorker(t, true) // td-1 still open, held on td-EPIC
	if err := ps.PutPR(store.PR{
		ID: "pr-td-EPIC", Task: "td-EPIC", Agent: "dain", Branch: "td-EPIC", Base: "main", Status: "merged", Kind: "interim",
	}); err != nil {
		t.Fatal(err)
	}

	dir, err := e.AgentDirective(context.Background(), "repo", "dain")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if _, has, _ := e.pendingMail("repo", "dain"); has {
		t.Fatal("no mail seeded; the assertion below assumes none")
	}
	// workDirective's own text names the held task; DirContainerDone (the wrong answer here) never
	// does. Checking state directly is the stronger assertion.
	if !strings.Contains(dir, "td-1") {
		t.Errorf("directive = %q, want the held subtask named — the interim merge must not have landed the feature", dir)
	}
	if st, _ := ps.GetState("dain"); st.Container != "td-EPIC" || st.Task != "td-1" {
		t.Errorf("state = {container:%q task:%q}, want the feature and its subtask both still held", st.Container, st.Task)
	}
}

// TestAFinalMergeDoesLandTheFeature is the control: a non-interim merge — the real submit — must
// still release the worker, exactly as before this fix. Once released, AgentDirective blocks
// waiting for the next claimable task (there is none in this fixture), so the release itself —
// the SetState clearing Container, written before that wait begins — is checked against a context
// cancelled almost immediately, rather than waiting the call out.
func TestAFinalMergeDoesLandTheFeature(t *testing.T) {
	e, ps, _, _ := featureWorker(t, true)
	if err := ps.PutPR(store.PR{
		ID: "pr-td-EPIC", Task: "td-EPIC", Agent: "dain", Branch: "td-EPIC", Base: "main", Status: "merged",
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := e.AgentDirective(ctx, "repo", "dain"); err != context.DeadlineExceeded {
		t.Fatalf("AgentDirective: %v, want the wait for a next task to time out (none is claimable)", err)
	}
	if st, _ := ps.GetState("dain"); st.Container != "" {
		t.Errorf("state.Container = %q, want cleared — a real merge must still release the worker", st.Container)
	}
}
