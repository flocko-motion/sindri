package task

import (
	prflow "github.com/flo-at/sindri/internal/hub/flow/pr"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/situation"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// armedWorker is ClaimNext's happy path (-> idleWorkerWithOpenTask: a real worktree, one open
// prioritized leaf) with a clear armed on the worker. Claimable work is the point: a gate tested
// against an empty queue would pass however it was written.
func armedWorker(t *testing.T) (*Act, *store.ProjectStore, *flowtest.Hub) {
	t.Helper()
	deps := &flowtest.Hub{}
	a, ps := idleWorkerWithOpenTask(t, deps)
	ag, _, _ := ps.GetAgent("dvalin")
	ag.ClearArmed = true
	if err := ps.PutAgent(ag); err != nil {
		t.Fatalf("arm the clear: %v", err)
	}
	return a, ps, deps
}

// TestArmedClearWithholdsTheNextTask is the half that stops a clear landing mid-work: an armed agent
// is handed nothing, so the boundary it stands at is still a boundary when the clear fires. Without
// it the agent asks, is given a task, and the clear meets the very guard it was waiting for.
func TestArmedClearWithholdsTheNextTask(t *testing.T) {
	a, ps, _ := armedWorker(t)
	if !a.ClearArmedFor("repo", "dvalin") {
		t.Fatal("the gate must read the arming the store holds")
	}
	if d, claimed, err := a.ClaimNext(t.Context(), "repo", "dvalin"); err != nil || claimed {
		t.Errorf("an armed agent was handed %q: claimed=%v err=%v", d, claimed, err)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("it was put on %q — the clear would land in the middle of it", st.Task)
	}
	// Disarmed, the same worker takes the same task: the withholding is the arming's doing and
	// nothing else's, which is what makes the case above a real gate.
	ag, _, _ := ps.GetAgent("dvalin")
	ag.ClearArmed = false
	if err := ps.PutAgent(ag); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := a.ClaimNext(t.Context(), "repo", "dvalin"); err != nil || !claimed {
		t.Errorf("with the arming gone the task should be claimable: claimed=%v err=%v", claimed, err)
	}
}

// TestArmedClearOutranksFullness is the interaction the two rules must get right: a human's own
// arming fires ahead of the automatic fullness path in waitForNextTask's own closure — else the
// arming sits behind a state that never advances.
func TestArmedClearOutranksFullness(t *testing.T) {
	a, _, deps := armedWorker(t)
	deps.CtxTokens, deps.CtxWindow, deps.CtxOK = 190_000, 200_000, true
	if _, full := a.agent().ContextFull("repo", "dvalin"); !full {
		t.Fatal("the stub should read as full — this interaction only exists for a full agent")
	}
	fired, err := a.agent().FireClearIfArmed(t.Context(), "repo", "dvalin")
	if err != nil || !fired {
		t.Fatalf("fireClearIfArmed = (%v, %v), want it to fire even though the agent also reads full", fired, err)
	}
	if len(deps.Cleared) != 1 || deps.Cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one Clear(dvalin)", deps.Cleared)
	}
	if len(deps.InjectedText) != 1 || deps.InjectedText[0] != prompts.MsgKickoff {
		t.Errorf("injectedText = %v, want the generic kickoff — nothing was claimed for this arming to hand over", deps.InjectedText)
	}
}

// TestAnArmedReviewerIsNotHandedTheNextPR closes the door the sweep's gate left open: reviews are also
// handed out by freeReviewer on the request path (RequestReview), which must gate the same arming.
func TestAnArmedReviewerIsNotHandedTheNextPR(t *testing.T) {
	armed := situation.Situation{Name: "fili", Role: "reviewer", ClearArmed: true}
	if prflow.ReviewerAssignable(armed) {
		t.Error("an armed reviewer must not be a candidate — the clear is waiting for it to be free")
	}
	free := armed
	free.ClearArmed = false
	if !prflow.ReviewerAssignable(free) {
		t.Error("disarmed, the same reviewer takes reviews again")
	}
	if prflow.ReviewerAssignable(situation.Situation{Name: "dvalin", Role: "worker"}) {
		t.Error("only reviewers review")
	}
	// Retirement disqualifies one too, which is what folding this into the surface bought: the two
	// used to be checked in different functions, and one of them forgot.
	retired := situation.Situation{Name: "fili", Role: "reviewer", Retired: true}
	if prflow.ReviewerAssignable(retired) {
		t.Error("a retired reviewer must not be a candidate either")
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
	a := newActWith2(t, st, &flowtest.Hub{Root: root, Alive: true})
	if err := a.pr().RequestReview("repo", "pr-c", ""); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}
	if holder, _ := ps.ReviewingPR("nori"); holder != "pr-c" {
		t.Errorf("nori is free and up — it should hold pr-c, got %q", holder)
	}
	if holder, _ := ps.ReviewingPR("fili"); holder != "" {
		t.Errorf("fili is armed for a clear and must not be handed a review, got %q", holder)
	}
}
