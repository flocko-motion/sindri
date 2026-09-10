package fleet

import (
	"github.com/flo-at/sindri/internal/api"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// plannerEngine seeds one flat proposal and the planner that authored it, for the tests that ask
// what the MACHINE tells a planner about it.
func plannerEngine(t *testing.T, id, approval string) (*Engine, registry.Caller, *store.ProjectStore) {
	t.Helper()
	e := storelessEngine(t, &stubDeps{Root: t.TempDir()})
	if err := e.Store.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ps := e.Store.For("proj")
	if err := ps.UpsertTask(store.Task{ID: id, Title: "flat proposal", Status: "open"}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	if approval != "" {
		if err := ps.SetApproval(id, approval, ""); err != nil {
			t.Fatalf("set approval: %v", err)
		}
	}
	return e, registry.Caller{Project: "proj", Agent: "galar", Role: "planner"}, ps
}

// plannerOwnedTask is plannerEngine with one task the planner itself authored, at whatever verdict
// the caller asks for — the shape every "may it still edit this" question starts from.
func plannerOwnedTask(t *testing.T, approval string) (*Engine, registry.Caller, *store.ProjectStore, string, *stubDeps) {
	t.Helper()
	e, c, ps := plannerEngine(t, "td-parent", "")
	id, err := e.taskAct().CreateTask("proj", api.TaskSpec{Title: "flat proposal", Description: "as first written", Type: "task"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if approval != "" {
		if err := ps.SetApproval(id, approval, ""); err != nil {
			t.Fatalf("set approval: %v", err)
		}
	}
	return e, c, ps, id, e.Deps.(*stubDeps)
}
