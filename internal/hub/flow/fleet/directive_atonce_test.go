package fleet

import (
	"context"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/world/store"
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
	if !strings.Contains(d, prompts.DirNoTasks) {
		t.Errorf("directive = %q, want prompts.DirNoTasks", d)
	}
}

// TestAgentDirectiveAnswersAtOnceForAnIdleReviewer is the same fix on the role AssignPendingReviews
// was already covering — ReviewDirective must answer immediately too, not just its periodic backstop.
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
	if !strings.Contains(d, prompts.DirNoReviews) {
		t.Errorf("directive = %q, want prompts.DirNoReviews", d)
	}
}

// TestTheDispatcherAssignsRatherThanAdvertises replaced a test of the opposite. The old design
// pushed "sd-… is ready for you" at an idle worker and left the claim to the worker's own next ask,
// because a claim made on its behalf could race that ask. The machine holds one action per subject,
// so that race is gone by construction — and a dispatcher that only spreads rumours about jobs
// maybe waiting is not a dispatcher.
func TestTheDispatcherAssignsRatherThanAdvertises(t *testing.T) {
	deps := &stubDeps{Alive: true}
	e, ps := idleWorkerWithOpenTask(t, deps)

	e.LookProject("repo")

	st, _ := ps.GetState("dvalin")
	if st.Task != "td-abc123" {
		t.Fatalf("state = %+v, want the task Assigned outright", st)
	}
	if st.Phase != worker.Working {
		t.Errorf("phase = %q, want %q", st.Phase, worker.Working)
	}
	if len(deps.InjectedText) == 0 || !strings.Contains(deps.InjectedText[0], "td-abc123") {
		t.Errorf("the brief for the work it was given must reach it: %v", deps.InjectedText)
	}
	if last := deps.Delivered[len(deps.Delivered)-1]; last.Mail || !last.Push {
		t.Errorf("the brief is push-only, got %+v", last)
	}
}

// TestAssignPendingWorkLeavesABusyWorkerAlone: the sweep must never claim on top of a worker that
// already holds something, whether a plain task or a feature between subtasks.
func TestAssignPendingWorkLeavesABusyWorkerAlone(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	// A REAL container row: a held feature whose task cannot be found reads as landed, and the
	// machine's answer to that is to release it — correctly, and not what this case is about.
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-EPIC", Title: "a feature", Status: "open", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	if err := e.taskAct().RefreshTask("repo", "td-EPIC"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Container: "td-EPIC", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}

	e.LookProject("repo")

	if st, _ := ps.GetState("dvalin"); st.Task != "" || st.Container != "td-EPIC" {
		t.Errorf("a worker already holding a feature must not be claimed onto a plain task, got %+v", st)
	}
	if len(deps.Delivered) != 0 {
		t.Errorf("nothing should have been pushed, got: %v", deps.InjectedText)
	}
}

// TestAnApprovalReachesTheFeatureWorkerWithoutASweep is the finding a review of this feature raised,
// restated for a dispatcher: the gated wait promised a push once the user ruled, and nothing sent
// one — leaving the stall watchdog to re-serve the same wait every dwell. The approval is now an
// event the worker's own map watches, and clearing the gate hands the subtask over rather than
// announcing that one might be available.
func TestAnApprovalReachesTheFeatureWorkerWithoutASweep(t *testing.T) {
	e, ps, _ := gatedFeature(t)
	if err := e.prAct().SetStatus("repo", "td-1", "closed"); err != nil {
		t.Fatal(err)
	}
	if err := e.taskAct().RefreshTask("repo", "td-1"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	deps := e.Deps.(*stubDeps)

	e.LookProject("repo")
	if len(deps.Injected) != 0 {
		t.Fatalf("still gated — nothing should be pushed yet, got %v", deps.InjectedText)
	}

	if err := e.taskAct().ApproveTask("repo", "td-2", false); err != nil {
		t.Fatalf("ApproveTask: %v", err)
	}
	e.LookProject("repo")
	if len(deps.Injected) != 1 || deps.Injected[0] != "dain" {
		t.Fatalf("once approved, dain should be handed the subtask, got %v", deps.Injected)
	}
	if !strings.Contains(deps.InjectedText[0], "td-2") {
		t.Errorf("the brief should name the subtask it was given: %s", deps.InjectedText[0])
	}
	if st, _ := ps.GetState("dain"); st.Task != "td-2" {
		t.Errorf("task = %q, want td-2 held — the approval assigns it rather than advertising it", st.Task)
	}
}

// TestAssignPendingSubtaskDoesNotRepeatIdenticalNudges: it shares notifyOnce's last_nudge slot with
// the plain-task path, and a sweep still finding the same subtask unclaimed must not repeat itself.
func TestAssignPendingSubtaskDoesNotRepeatIdenticalNudges(t *testing.T) {
	e, ps, _ := gatedFeature(t)
	if err := e.prAct().SetStatus("repo", "td-1", "closed"); err != nil {
		t.Fatal(err)
	}
	if err := e.taskAct().RefreshTask("repo", "td-1"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if err := e.taskAct().ApproveTask("repo", "td-2", false); err != nil {
		t.Fatalf("ApproveTask: %v", err)
	}
	deps := e.Deps.(*stubDeps)

	e.LookProject("repo")
	e.LookProject("repo")
	if len(deps.Injected) != 1 {
		t.Fatalf("the same unclaimed subtask must be pushed once, not on every sweep, got %v", deps.Injected)
	}
}

// TestAssignPendingSubtaskDoesNotPushAPendingClearNotice: an armed clear is a wait of its own, not
// news to push, and FireArmedClears (later in the same hub tick) is what actually fires it.
func TestAssignPendingSubtaskDoesNotPushAPendingClearNotice(t *testing.T) {
	e, ps, _ := gatedFeature(t)
	if err := e.prAct().SetStatus("repo", "td-1", "closed"); err != nil {
		t.Fatal(err)
	}
	if err := e.taskAct().RefreshTask("repo", "td-1"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	a, _, _ := ps.GetAgent("dain")
	a.ClearArmed = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	deps := e.Deps.(*stubDeps)

	e.LookProject("repo")

	if len(deps.Injected) != 0 {
		t.Errorf("a pending-clear notice must not be pushed to an agent that never asked, got %v", deps.InjectedText)
	}
	if st, _ := ps.GetState("dain"); st.Task != "" {
		t.Errorf("nothing should be claimed while a clear is armed, got task=%q", st.Task)
	}
}
