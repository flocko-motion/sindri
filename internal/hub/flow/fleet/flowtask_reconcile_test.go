package fleet

import (
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestReconcileRepairsAStaleInProgress: a task marked in_progress that nobody holds is stale, which
// is the whole reason the sweep exists.
func TestReconcileRepairsAStaleInProgress(t *testing.T) {
	e, ps, id := ownedEngine(t, "in_progress")
	if err := e.taskAct().ReconcileTasks("proj"); err != nil {
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
	if err := e.taskAct().ReconcileTasks("proj"); err != nil {
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
	flowtest.Place(t, ps, store.AgentState{Agent: "eitri", Task: id, Branch: id, Phase: "working"})
	if err := e.taskAct().ReconcileTasks("proj"); err != nil {
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
	if err := e.taskAct().RefreshTask("proj", id); err != nil {
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

// TestSyncTasksCarriesOwnedUpdatedAt: a full sync goes owned_tasks -> owned.OwnedSource.Tasks ->
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
	if err := e.taskAct().SyncTasks("proj"); err != nil {
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
