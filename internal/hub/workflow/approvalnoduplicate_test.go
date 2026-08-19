package workflow

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// approvalWithPlanner seeds one pending task and a running planner to notify — notifyPlanners has
// nobody to tell without one, which would make every test here pass vacuously.
func approvalWithPlanner(t *testing.T) (*Engine, *store.ProjectStore, *stubDeps) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "nabbi", Role: "planner"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Status: "open", Title: "a task", Priority: "P1"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := ps.SetApproval("td-1", "pending", ""); err != nil {
		t.Fatalf("gate: %v", err)
	}
	deps := &stubDeps{root: t.TempDir()}
	return New(st, deps), ps, deps
}

// settle gives notifyPlanners' fire-and-forget goroutine a moment to land: the verb itself never
// blocks on delivery, so the test has to wait for it rather than reading deps the instant it returns.
func settle() { time.Sleep(50 * time.Millisecond) }

// TestApprovingAnAlreadyApprovedTaskDoesNotReannounce is sd-dde3b6: the message describes a
// transition, so a second approval of the same task — already approved, nothing pending below it —
// has nothing to announce, even though the verb itself must still succeed.
func TestApprovingAnAlreadyApprovedTaskDoesNotReannounce(t *testing.T) {
	e, ps, deps := approvalWithPlanner(t)

	if err := e.ApproveTask("proj", "td-1", false); err != nil {
		t.Fatalf("first ApproveTask: %v", err)
	}
	settle()
	if len(deps.injected) != 1 {
		t.Fatalf("the first approval should notify once, got %d", len(deps.injected))
	}

	if err := e.ApproveTask("proj", "td-1", false); err != nil {
		t.Fatalf("second ApproveTask: %v", err)
	}
	settle()
	if len(deps.injected) != 1 {
		t.Errorf("approving an already-approved task must not re-announce, got %d message(s): %v", len(deps.injected), deps.injectedText)
	}
	if got, _ := ps.GetApproval("td-1"); got != "approved" {
		t.Errorf("the verb should still succeed, gate = %q", got)
	}
}

// TestApprovingASubtreeStillAnnouncesWhatItReleases: the root can already be approved while a child
// added since is still pending — that IS a real change, so it must still be announced (only a total
// no-op stays silent).
func TestApprovingASubtreeStillAnnouncesWhatItReleases(t *testing.T) {
	e, ps, deps := approvalWithPlanner(t)
	if err := e.ApproveTask("proj", "td-1", false); err != nil {
		t.Fatalf("ApproveTask: %v", err)
	}
	settle()
	if err := ps.UpsertTask(store.Task{ID: "td-2", Status: "open", Title: "added later", ParentID: "td-1"}); err != nil {
		t.Fatalf("seed td-2: %v", err)
	}
	if err := ps.SetApproval("td-2", "pending", ""); err != nil {
		t.Fatalf("gate td-2: %v", err)
	}

	if err := e.ApproveTask("proj", "td-1", true); err != nil {
		t.Fatalf("subtree ApproveTask: %v", err)
	}
	settle()
	if len(deps.injected) != 2 {
		t.Errorf("td-1 already approved but td-2 was still pending — that release must still be announced, got %d message(s): %v",
			len(deps.injected), deps.injectedText)
	}
}

// TestRejectingAnAlreadyRejectedTaskDoesNotReannounce is RejectTask's half of the same fix.
func TestRejectingAnAlreadyRejectedTaskDoesNotReannounce(t *testing.T) {
	e, ps, deps := approvalWithPlanner(t)

	if err := e.RejectTask("proj", "td-1", "not ready"); err != nil {
		t.Fatalf("first RejectTask: %v", err)
	}
	settle()
	if len(deps.injected) != 1 {
		t.Fatalf("the first rejection should notify once, got %d", len(deps.injected))
	}

	if err := e.RejectTask("proj", "td-1", "not ready"); err != nil {
		t.Fatalf("second RejectTask: %v", err)
	}
	settle()
	if len(deps.injected) != 1 {
		t.Errorf("rejecting an already-rejected task with the same comment must not re-announce, got %d: %v",
			len(deps.injected), deps.injectedText)
	}
	if status, comment := ps.GetApproval("td-1"); status != "rejected" || comment != "not ready" {
		t.Errorf("the verb should still succeed, got status=%q comment=%q", status, comment)
	}
}

// TestRejectingAgainWithNewFeedbackStillAnnounces: a second rejection is only a no-op when it
// repeats itself — different feedback is new information for the planner to see.
func TestRejectingAgainWithNewFeedbackStillAnnounces(t *testing.T) {
	e, _, deps := approvalWithPlanner(t)

	if err := e.RejectTask("proj", "td-1", "not ready"); err != nil {
		t.Fatalf("first RejectTask: %v", err)
	}
	settle()
	if err := e.RejectTask("proj", "td-1", "wrong approach entirely"); err != nil {
		t.Fatalf("second RejectTask: %v", err)
	}
	settle()
	if len(deps.injected) != 2 {
		t.Errorf("different feedback on a second rejection should still announce, got %d: %v", len(deps.injected), deps.injectedText)
	}
}
