// package: hub/flow/fleet / directive_cleararmed_test
// type:    logic (when an armed clear is allowed to land)
// job:     pin the boundary — a clear armed mid-subtask waits for the checkpoint, and fires at the
// leaf boundary the directive next answers from.
// limits:  the timing, which only a directive pass can show. What an armed clear WITHHOLDS is
// tested beside the claim (-> flow/task's task_act_cleararmed_test.go).
package fleet

import (
	"context"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
	"testing"
)

// TestTheClearLandsBeforeTheNextSubtask: mid-subtask the clear waits; between subtasks, where a
// checkpoint leaves it, the worker's own clearing state fires it — the same route an idle one takes.
func TestTheClearLandsBeforeTheNextSubtask(t *testing.T) {
	e, ps, deps := containerWorker(t, "working")
	// A session with something in it: a clear typed into an empty one never lands (-> cond.ClearArmed).
	deps.CtxTokens, deps.CtxWindow, deps.CtxOK = 80_000, 200_000, true
	ag, _, _ := ps.GetAgent("dvalin")
	ag.ClearArmed = true
	if err := ps.PutAgent(ag); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AgentDirective(context.Background(), "repo", "dvalin"); err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if len(deps.Cleared) != 0 {
		t.Error("mid-subtask the agent keeps working; the clear must not fire before the checkpoint")
	}

	flowtest.Place(t, ps, store.AgentState{Agent: "dvalin", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"})
	e.Look("repo", "dvalin")
	if len(deps.Cleared) != 1 || deps.Cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want the clear fired at the subtask boundary", deps.Cleared)
	}
}

// TestAnArmedClearFiresBeforeTheClaim is the half that stops a clear landing mid-work: the arming is
// answered first and the work is handed over behind it, so nothing is ever given to a session that
// is about to be discarded. It used to WITHHOLD the task instead, which left a human's arming waiting
// on work that would never be claimed while it waited.
func TestAnArmedClearFiresBeforeTheClaim(t *testing.T) {
	deps := &stubDeps{CtxTokens: 80_000, CtxWindow: 200_000, CtxOK: true}
	e, ps := idleWorkerWithOpenTask(t, deps)
	ag, _, _ := ps.GetAgent("dvalin")
	ag.ClearArmed = true
	if err := ps.PutAgent(ag); err != nil {
		t.Fatal(err)
	}

	e.Look("repo", "dvalin")

	if len(deps.Cleared) != 1 || deps.Cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one Clear(dvalin) ahead of the hand-over", deps.Cleared)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want the task claimed behind the clear", st.Task)
	}
	if ag, _, _ := ps.GetAgent("dvalin"); ag.ClearArmed {
		t.Error("the arming must be answered, or it brings the agent back here on every beat")
	}
}
