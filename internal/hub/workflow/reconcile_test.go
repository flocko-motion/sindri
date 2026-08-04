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

// TestReconciledStatusRule pins the pure rule the sweep applies.
func TestReconciledStatusRule(t *testing.T) {
	for _, c := range []struct {
		status             string
		activePR, assigned bool
		want               string
	}{
		{"in_progress", false, false, "open"},       // nobody holds it
		{"in_progress", false, true, "in_progress"}, // its worker still does
		{"in_review", false, false, "open"},         // no PR, nobody holds it
		{"in_review", false, true, "in_progress"},   // no PR, but still assigned
		{"in_review", true, false, "in_review"},     // a live PR justifies it
		{"closed", false, false, "closed"},          // a finished task is left alone
		{"open", false, false, "open"},
	} {
		if got := reconciledStatus(c.status, c.activePR, c.assigned); got != c.want {
			t.Errorf("reconciledStatus(%q, pr=%v, assigned=%v) = %q, want %q",
				c.status, c.activePR, c.assigned, got, c.want)
		}
	}
}
