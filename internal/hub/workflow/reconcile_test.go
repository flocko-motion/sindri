package workflow

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// ownedEngine returns an engine over a throwaway store with one project and one owned task.
func ownedEngine(t *testing.T, status string) (*Engine, *store.ProjectStore, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	const id = "td-abc123"
	if err := ps.PutOwnedTask(store.OwnedTask{ID: id, Title: "a task", Status: status, Priority: "P2"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	return New(st, &stubDeps{root: root}), ps, id
}

// TestReconcileRepairsAStaleInProgress: a task marked in_progress that nobody holds is stale, which
// is the whole reason the sweep exists.
func TestReconcileRepairsAStaleInProgress(t *testing.T) {
	e, ps, id := ownedEngine(t, "in_progress")
	if err := e.ReconcileTasks("proj"); err != nil {
		t.Fatalf("ReconcileTasks: %v", err)
	}
	got, _, _ := ps.OwnedTask(id)
	if got.Status != "open" {
		t.Errorf("status %q, want open — in_progress with no assignee is stale", got.Status)
	}
}

// TestReconcileLeavesAClosedTaskAlone: closing is what a merge does, and reopening it sent a worker
// back onto work it had just finished. With one store there is no lagging mirror to decide from, so
// this holds by construction rather than by reading the status from the right place.
func TestReconcileLeavesAClosedTaskAlone(t *testing.T) {
	e, ps, id := ownedEngine(t, "closed")
	if err := e.ReconcileTasks("proj"); err != nil {
		t.Fatalf("ReconcileTasks: %v", err)
	}
	got, _, _ := ps.OwnedTask(id)
	if got.Status != "closed" {
		t.Errorf("status %q, want closed — a finished task must stay finished", got.Status)
	}
}

// TestReconcileKeepsAnAssignedTaskInProgress: the agent holding it is the justification, so the
// sweep must not free work that is genuinely underway.
func TestReconcileKeepsAnAssignedTaskInProgress(t *testing.T) {
	e, ps, id := ownedEngine(t, "in_progress")
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Task: id, Branch: id, Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if err := e.ReconcileTasks("proj"); err != nil {
		t.Fatalf("ReconcileTasks: %v", err)
	}
	got, _, _ := ps.OwnedTask(id)
	if got.Status != "in_progress" {
		t.Errorf("status %q, want in_progress — eitri holds it", got.Status)
	}
}

// TestRefreshTaskCarriesUpdatedAt: the "active" tasks filter reads UpdatedAt off the cached row, so
// a status change must reach it there too, not just owned_tasks — RefreshTask is what every claim,
// release and reconcile path calls right after SetOwnedStatus to keep the two in step.
func TestRefreshTaskCarriesUpdatedAt(t *testing.T) {
	e, ps, id := ownedEngine(t, "open")
	if err := ps.SetOwnedStatus(id, "in_progress"); err != nil {
		t.Fatal(err)
	}
	owned, _, _ := ps.OwnedTask(id)
	if owned.UpdatedAt == "" {
		t.Fatal("precondition: SetOwnedStatus should have stamped owned_tasks.updated_at")
	}
	if err := e.RefreshTask("proj", id); err != nil {
		t.Fatal(err)
	}
	cached, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if cached.UpdatedAt != owned.UpdatedAt {
		t.Errorf("cached UpdatedAt = %q, want the owned row's %q", cached.UpdatedAt, owned.UpdatedAt)
	}
}

// TestSyncTasksCarriesOwnedUpdatedAt: a full sync goes owned_tasks -> ownedSource.Tasks ->
// ToStoreTask -> ReplaceTasks, a different path than RefreshTask's single-row read — each hop
// dropped UpdatedAt until it did, so this pins the chain end to end rather than just one link.
func TestSyncTasksCarriesOwnedUpdatedAt(t *testing.T) {
	e, ps, id := ownedEngine(t, "open")
	if err := ps.SetOwnedStatus(id, "in_progress"); err != nil {
		t.Fatal(err)
	}
	owned, _, _ := ps.OwnedTask(id)
	if owned.UpdatedAt == "" {
		t.Fatal("precondition: SetOwnedStatus should have stamped owned_tasks.updated_at")
	}
	if err := e.SyncTasks("proj"); err != nil {
		t.Fatal(err)
	}
	cached, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		t.Fatalf("GetTask: ok=%v err=%v", ok, err)
	}
	if cached.UpdatedAt != owned.UpdatedAt {
		t.Errorf("cached UpdatedAt = %q after a full sync, want the owned row's %q", cached.UpdatedAt, owned.UpdatedAt)
	}
}

// TestReconciledStatusRule pins the pure rule the sweep applies.
func TestReconciledStatusRule(t *testing.T) {
	for _, c := range []struct {
		status                           string
		activePR, assigned, openChildren bool
		want                             string
	}{
		{"in_progress", false, false, false, "open"},       // nobody holds it
		{"in_progress", false, true, false, "in_progress"}, // its worker still does
		{"in_review", false, false, false, "open"},         // no PR, nobody holds it
		{"in_review", false, true, false, "in_progress"},   // no PR, but still assigned
		{"in_review", true, false, false, "in_review"},     // a live PR justifies it
		{"closed", false, false, false, "closed"},          // a finished task is left alone
		{"open", false, false, false, "open"},
		// A parent is finished exactly when its children are, so a done one with work still open
		// under it is stale in the same way the others are — and is reopened, which is what heals a
		// tree an earlier build broke by closing the middle of it.
		{"closed", false, false, true, "open"},
		{"merged", false, false, true, "open"},
		{"approved", false, false, true, "open"},
		{"open", false, false, true, "open"}, // already open: having children changes nothing
	} {
		if got := reconciledStatus(c.status, c.activePR, c.assigned, c.openChildren); got != c.want {
			t.Errorf("reconciledStatus(%q, pr=%v, assigned=%v, openChildren=%v) = %q, want %q",
				c.status, c.activePR, c.assigned, c.openChildren, got, c.want)
		}
	}
}
