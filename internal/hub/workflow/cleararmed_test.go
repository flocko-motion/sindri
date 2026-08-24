package workflow

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// armedWorker is claimNext's happy path (-> idleWorkerWithOpenTask: a real worktree, one open
// prioritized leaf) with a clear armed on the worker. Claimable work is the point: a gate tested
// against an empty queue would pass however it was written.
func armedWorker(t *testing.T) (*Engine, *store.ProjectStore, *stubDeps) {
	t.Helper()
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	a, _, _ := ps.GetAgent("dvalin")
	a.ClearArmed = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatalf("arm the clear: %v", err)
	}
	return e, ps, deps
}

// TestArmedClearWithholdsTheNextTask is the half that stops a clear landing mid-work: an armed agent
// is handed nothing, so the boundary it stands at is still a boundary when the clear fires. Without
// it the agent asks, is given a task, and the clear meets the very guard it was waiting for.
func TestArmedClearWithholdsTheNextTask(t *testing.T) {
	e, ps, _ := armedWorker(t)
	if !e.clearArmed("repo", "dvalin") {
		t.Fatal("the gate must read the arming the store holds")
	}
	if d, claimed, err := e.claimNext("repo", "dvalin"); err != nil || claimed {
		t.Errorf("an armed agent was handed %q: claimed=%v err=%v", d, claimed, err)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("it was put on %q — the clear would land in the middle of it", st.Task)
	}
	// Disarmed, the same worker takes the same task: the withholding is the arming's doing and
	// nothing else's, which is what makes the case above a real gate.
	a, _, _ := ps.GetAgent("dvalin")
	a.ClearArmed = false
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := e.claimNext("repo", "dvalin"); err != nil || !claimed {
		t.Errorf("with the arming gone the task should be claimable: claimed=%v err=%v", claimed, err)
	}
}

// TestArmedClearOutranksFullness is the interaction the two rules must get right: a human's own
// arming fires ahead of the automatic fullness path in waitForNextTask's own closure — else the
// arming sits behind a state that never advances.
func TestArmedClearOutranksFullness(t *testing.T) {
	e, _, deps := armedWorker(t)
	deps.ctxTokens, deps.ctxWindow, deps.ctxOK = 190_000, 200_000, true
	if _, full := e.contextFull("repo", "dvalin"); !full {
		t.Fatal("the stub should read as full — this interaction only exists for a full agent")
	}
	fired, err := e.fireClearIfArmed("repo", "dvalin")
	if err != nil || !fired {
		t.Fatalf("fireClearIfArmed = (%v, %v), want it to fire even though the agent also reads full", fired, err)
	}
	if len(deps.cleared) != 1 || deps.cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one FireClear(dvalin)", deps.cleared)
	}
	if len(deps.clearedWith) != 1 || deps.clearedWith[0] != MsgKickoff {
		t.Errorf("clearedWith = %v, want the generic kickoff — nothing was claimed for this arming to hand over", deps.clearedWith)
	}
	// fireClearIfArmed runs inside the call answering this very ask, same as the automatic path —
	// ESC here would cut off the turn computing whatever this ask answers with.
	if len(deps.clearedInterrupt) != 1 || deps.clearedInterrupt[0] {
		t.Errorf("clearedInterrupt = %v, want false — this fires inside the agent's own ask", deps.clearedInterrupt)
	}
}

// TestAnArmedReviewerIsNotHandedTheNextPR closes the door the sweep's gate left open: reviews are also
// handed out by freeReviewer on the request path (RequestReview), which must gate the same arming.
func TestAnArmedReviewerIsNotHandedTheNextPR(t *testing.T) {
	armed := store.Agent{Name: "fili", Role: "reviewer", ClearArmed: true}
	if reviewerAssignable(armed) {
		t.Error("an armed reviewer must not be a candidate — the clear is waiting for it to be free")
	}
	free := armed
	free.ClearArmed = false
	if !reviewerAssignable(free) {
		t.Error("disarmed, the same reviewer takes reviews again")
	}
	if reviewerAssignable(store.Agent{Name: "dvalin", Role: "worker"}) {
		t.Error("only reviewers review")
	}
}

// TestAnArmedReviewerIsNotHandedTheNextPRThroughRequestReview drives the request path end to end:
// freeReviewer's liveness check reads AgentUp (the watchdog's own reading), so a stub states it
// directly with no container runtime required.
func TestAnArmedReviewerIsNotHandedTheNextPRThroughRequestReview(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "fili", Role: "reviewer", Workspace: ".worktrees/fili", ClearArmed: true}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutAgent(store.Agent{Name: "nori", Role: "reviewer", Workspace: ".worktrees/nori"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-c", Task: "td-c", Agent: "bombur", Branch: "pr-c", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: root, alive: true})
	if err := e.RequestReview("repo", "pr-c", ""); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}
	if holder, _ := ps.ReviewingPR("nori"); holder != "pr-c" {
		t.Errorf("nori is free and up — it should hold pr-c, got %q", holder)
	}
	if holder, _ := ps.ReviewingPR("fili"); holder != "" {
		t.Errorf("fili is armed for a clear and must not be handed a review, got %q", holder)
	}
}

// TestTheClearLandsBeforeTheNextSubtask: mid-subtask the clear waits; between subtasks, where a
// checkpoint leaves it, fireClearIfArmed fires it, same as the idle worker's own path.
func TestTheClearLandsBeforeTheNextSubtask(t *testing.T) {
	e, ps, deps := containerWorker(t, "working")
	a, _, _ := ps.GetAgent("dvalin")
	a.ClearArmed = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AgentDirective(context.Background(), "repo", "dvalin"); err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if len(deps.cleared) != 0 {
		t.Error("mid-subtask the agent keeps working; the clear must not fire before the checkpoint")
	}

	if err := ps.SetState(store.AgentState{Agent: "dvalin", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	fired, err := e.fireClearIfArmed("repo", "dvalin")
	if err != nil || !fired {
		t.Fatalf("fireClearIfArmed = (%v, %v), want it to fire now the agent is between subtasks", fired, err)
	}
}
