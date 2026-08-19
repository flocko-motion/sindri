package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// answersAtOnce runs check in a goroutine and fails the test if it does not return within a couple
// of seconds — sd-ea017f's whole point: `sindri` must never hold the request open waiting for work.
func answersAtOnce(t *testing.T, check func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		check()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("did not answer at once — something is still blocking until work appears")
	}
}

// TestAgentDirectiveAnswersAtOnceWithNoWork: an idle worker beside an empty backlog gets a truthful
// wait immediately, not a held request — AssignPendingWork is what pushes it a wake once there is
// something to claim.
func TestAgentDirectiveAnswersAtOnceWithNoWork(t *testing.T) {
	e, ps := runEngine(t)
	if err := ps.PutAgent(store.Agent{Name: "wrk", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	var d string
	var err error
	answersAtOnce(t, func() { d, err = e.AgentDirective(context.Background(), "repo", "wrk") })
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if d != DirNoTasks {
		t.Errorf("directive = %q, want DirNoTasks", d)
	}
}

// TestAgentDirectiveAnswersAtOnceForAnIdleReviewer is the same fix on the role AssignPendingReviews
// was already covering — reviewDirective must answer immediately too, not just its periodic backstop.
func TestAgentDirectiveAnswersAtOnceForAnIdleReviewer(t *testing.T) {
	e, ps := runEngine(t)
	if err := ps.PutAgent(store.Agent{Name: "rev", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	var d string
	var err error
	answersAtOnce(t, func() { d, err = e.AgentDirective(context.Background(), "repo", "rev") })
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if d != DirNoReviews {
		t.Errorf("directive = %q, want DirNoReviews", d)
	}
}

// TestAssignPendingWorkPushesForAnIdleWorker is the safety net a non-blocking `sindri` needs:
// nothing else tells a worker that stopped asking about a task that became claimable later, so the
// periodic sweep pushes a wake and leaves the claim itself to the worker's own next ask — a claim
// made here, on the worker's behalf, could race that ask (-> nextUp reading a stale open-task set
// twice) and strand the task in_progress with nobody holding it.
func TestAssignPendingWorkPushesForAnIdleWorker(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)

	e.AssignPendingWork("repo")

	st, _ := ps.GetState("dvalin")
	if st.Task != "" {
		t.Fatalf("state = %+v, want nothing claimed — the sweep pushes, the worker's own ask claims", st)
	}
	if len(deps.delivered) == 0 {
		t.Fatal("a wake should have been pushed to the worker")
	}
	if last := deps.delivered[len(deps.delivered)-1]; last.Mail || !last.Push {
		t.Errorf("the wake must be push-only, got %+v", last)
	}
	if !strings.Contains(deps.injectedText[len(deps.injectedText)-1], "td-abc123") {
		t.Errorf("the push should name what is claimable: %s", deps.injectedText[len(deps.injectedText)-1])
	}
}

// TestAssignPendingWorkLeavesABusyWorkerAlone: the sweep must never claim on top of a worker that
// already holds something, whether a plain task or a feature between subtasks.
func TestAssignPendingWorkLeavesABusyWorkerAlone(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Container: "td-EPIC", Phase: "working"}); err != nil {
		t.Fatal(err)
	}

	e.AssignPendingWork("repo")

	if st, _ := ps.GetState("dvalin"); st.Task != "" || st.Container != "td-EPIC" {
		t.Errorf("a worker already holding a feature must not be claimed onto a plain task, got %+v", st)
	}
	if len(deps.delivered) != 0 {
		t.Errorf("nothing should have been pushed, got: %v", deps.injectedText)
	}
}

// TestAssignPendingWorkWakesAGatedFeatureWorkerOnceApproved is the finding a review of this
// feature raised: claimNextSubtask's gated wait (ReplyFeatureGated) promises a push once the user
// rules, but nothing sent one — leaving the stall watchdog to re-serve the same wait every dwell
// instead. The sweep pushes a wake once the gate clears; the worker's own next ask does the claim.
func TestAssignPendingWorkWakesAGatedFeatureWorkerOnceApproved(t *testing.T) {
	e, ps, _ := gatedFeature(t)
	if err := e.SetStatus("repo", "td-1", "closed"); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshTask("repo", "td-1"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	deps := e.deps.(*stubDeps)

	e.AssignPendingWork("repo")
	if len(deps.injected) != 0 {
		t.Fatalf("still gated — nothing should be pushed yet, got %v", deps.injectedText)
	}

	if err := e.ApproveTask("repo", "td-2", false); err != nil {
		t.Fatalf("ApproveTask: %v", err)
	}
	e.AssignPendingWork("repo")
	if len(deps.injected) != 1 || deps.injected[0] != "dain" {
		t.Fatalf("once approved, the sweep should wake dain, got %v", deps.injected)
	}
	if !strings.Contains(deps.injectedText[0], "td-2") {
		t.Errorf("the push should name the newly-claimable subtask: %s", deps.injectedText[0])
	}
	if st, _ := ps.GetState("dain"); st.Task != "" {
		t.Errorf("nothing should be claimed by the sweep, got task=%q — that stays dain's own ask", st.Task)
	}
}

// TestAssignPendingSubtaskDoesNotPushAPendingClearNotice: an armed clear is a wait of its own, not
// news to push, and FireArmedClears (later in the same hub tick) is what actually fires it.
func TestAssignPendingSubtaskDoesNotPushAPendingClearNotice(t *testing.T) {
	e, ps, _ := gatedFeature(t)
	if err := e.SetStatus("repo", "td-1", "closed"); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshTask("repo", "td-1"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	a, _, _ := ps.GetAgent("dain")
	a.ClearArmed = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	deps := e.deps.(*stubDeps)

	e.AssignPendingWork("repo")

	if len(deps.injected) != 0 {
		t.Errorf("a pending-clear notice must not be pushed to an agent that never asked, got %v", deps.injectedText)
	}
	if st, _ := ps.GetState("dain"); st.Task != "" {
		t.Errorf("nothing should be claimed while a clear is armed, got task=%q", st.Task)
	}
}
