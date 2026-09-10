// package: hub/flow/fleet / retire_fixture_test
// type:    logic (a retired worker with work waiting)
// job:     seed the shape a retirement is judged on — an agent up, a task open in the backlog —
// for the tests that ask what the DIRECTIVE tells it.
// limits:  seeding only.
package fleet

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// retireFixture is one worker and one open, claimable task.
func retireFixture(t *testing.T) (*Engine, *store.ProjectStore, registry.Caller) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "work", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	return newEngine(t, st, &stubDeps{Root: root, Alive: true}), ps,
		registry.Caller{Project: "proj", Agent: "dvalin", Role: "worker", Phase: "idle"}
}
