package workflow

import (
	"context"
	"strings"
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

// TestArmedAgentIsToldWhyItGetsNothing: it must not read "no open tasks", which is a claim about the
// queue and would leave the agent waiting for the wrong thing.
func TestArmedAgentIsToldWhyItGetsNothing(t *testing.T) {
	e, _, _ := armedWorker(t)
	d, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if d != DirClearPending {
		t.Errorf("directive = %q, want the pending-clear answer", d)
	}
	if !strings.Contains(DirClearPending, "context clear") {
		t.Errorf("the directive should name what is happening: %q", DirClearPending)
	}
	if strings.Contains(DirClearPending, "No open tasks") {
		t.Error("an armed agent is not idle for want of work")
	}
}

// TestArmedClearOutranksFullness is the interaction the two rules must get right: a full agent is
// retired from assignment "until a human clears you", so once one HAS, the answer must stop being
// "wait for a human" — else the arming sits behind a state that never advances.
func TestArmedClearOutranksFullness(t *testing.T) {
	e, _, deps := armedWorker(t)
	deps.ctxTokens, deps.ctxWindow, deps.ctxOK = 190_000, 200_000, true
	if _, full := e.contextFull("repo", "dvalin"); !full {
		t.Fatal("the stub should read as full — this interaction only exists for a full agent")
	}
	d, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if d != DirClearPending {
		t.Errorf("directive = %q, want the pending clear rather than the fullness notice", d)
	}
}

// TestTheClearLandsBeforeTheNextSubtask: mid-subtask the agent carries on and the clear waits (no
// path clears an agent mid-task); between subtasks — where a checkpoint leaves it — the clear takes
// precedence over the subtask that would otherwise be served next.
func TestTheClearLandsBeforeTheNextSubtask(t *testing.T) {
	e, ps, _ := containerWorker(t, "working")
	a, _, _ := ps.GetAgent("dvalin")
	a.ClearArmed = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	d, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if d == DirClearPending {
		t.Error("mid-subtask the agent keeps working; the clear waits for the checkpoint")
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	if d, err = e.AgentDirective(context.Background(), "repo", "dvalin"); err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if d != DirClearPending {
		t.Errorf("directive = %q, want the clear to land before the next subtask is served", d)
	}
}
