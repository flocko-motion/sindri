package fleet

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// ownedEngine seeds ONE task sindri owns, in both the owned table and the read model a sync leaves —
// the pair the repair sweep exists to keep in step. An ENGINE rather than a task's acting half: what
// the sweep repairs is decided by the task and PR maps, so there has to be a machine to run them.
func ownedEngine(t *testing.T, status string) (*Engine, *store.ProjectStore, string) {
	t.Helper()
	e := storelessEngine(t, &stubDeps{})
	if err := e.Store.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := e.Store.For("proj")
	const id = "td-abc123"
	if err := ps.PutOwnedTask(store.OwnedTask{ID: id, Title: "a task", Status: status, Priority: "P2"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := ps.UpsertTask(store.Task{ID: id, Title: "a task", Status: status, Priority: "P2"}); err != nil {
		t.Fatalf("seed cache row: %v", err)
	}
	return e, ps, id
}
