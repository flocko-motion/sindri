// package: hub/flow/fleet / quietworker_fixture_test
// type:    logic (a live worker whose screen has stopped)
// job:     seed the shape a stall is judged on — an agent up, holding a task, its pane still — for
// the tests that ask what the MACHINE does with one.
// limits:  seeding only. What a nudge SAYS is tested beside the rule (-> flow/agent's stall.go).
package fleet

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// quietWorkerHoldingWork seeds a live worker holding a task with its screen gone still — the shape
// that produced "nudge stalled on os-8ea68f — idle for 6m0s".
func quietWorkerHoldingWork(t *testing.T, deps *stubDeps) (*Engine, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	deps.Root, deps.Alive = root, true
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "sd-1", Branch: "sd-1", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	return newEngine(t, st, deps), ps
}
