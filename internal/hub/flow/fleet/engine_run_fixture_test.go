package fleet

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// runEngine seeds a worker with a workspace, for the tests that queue a run through the machine.
func runEngine(t *testing.T) (*Engine, *store.ProjectStore) {
	t.Helper()
	e := storelessEngine(t, &stubDeps{})
	if err := e.Store.RegisterProject("repo", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ps := e.Store.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatal(err)
	}
	return e, ps
}
