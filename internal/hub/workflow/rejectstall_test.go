package workflow

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestARejectedWorkerIsStillCaughtByTheStallNudge closes the gap a reviewer's own bug (sd-98fa96)
// raised for workers too: a rejection sets phase "working", not "idle", so the existing stall
// dwell already watches a worker that goes quiet after a verdict — unlike a reviewer's
// completeReview, which left phase "idle" and was invisible to it. No new invariant needed; this
// pins that the integration actually holds.
func TestARejectedWorkerIsStillCaughtByTheStallNudge(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: ".worktrees/bombur"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "bombur", Branch: "td-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "td-1", Branch: "td-1", Phase: "submitted"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: t.TempDir(), alive: true}
	e := New(st, deps)

	if err := e.RejectPR("proj", "pr-1", "needs another pass"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if got, _ := ps.GetState("bombur"); got.Phase != "working" {
		t.Fatalf("phase after rejection = %q, want working — that's what makes it visible to Stalled", got.Phase)
	}

	before := len(deps.injectedText)
	if !e.NudgeStalled("proj", "bombur", "idle", StallDwell+time.Minute) {
		t.Fatal("a rejected worker gone quiet past the dwell should be nudged")
	}
	if len(deps.injectedText) <= before {
		t.Fatal("NudgeStalled reported success but injected nothing new")
	}
}
