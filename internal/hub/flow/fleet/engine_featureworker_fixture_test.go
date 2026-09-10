// package: hub/flow/fleet / featureworker_fixture_test
// type:    logic (a worker holding a feature, on a real branch)
// job:     seed the one shape half these tests need — an agent holding a container with a subtask
// under it, its worktree on the feature branch with work in it — so a test about what LANDS starts
// from a repo that can actually be committed to.
// limits:  seeding. What the feature then does is each test's own.
package fleet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// featureWorker seeds dain holding td-EPIC, with td-1 under it open or closed as the caller asks.
func featureWorker(t *testing.T, openChild bool) (*Engine, *store.ProjectStore, registry.Caller, *stubDeps) {
	t.Helper()
	const agent = "dain"
	root, _ := flowtest.WorkRepo(t, agent, "td-EPIC")
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	child := store.Task{ID: "td-1", Title: "a subtask", Status: "closed", Priority: "P1", ParentID: "td-EPIC"}
	if openChild {
		child.Status = "open"
	}
	for _, task := range []store.Task{
		{ID: "td-EPIC", Title: "a feature", Status: "open", Priority: "P1", Type: "epic"}, child,
	} {
		if err := ps.UpsertTask(task); err != nil {
			t.Fatalf("seed %s: %v", task.ID, err)
		}
	}
	phase := "idle" // every subtask checkpointed: the feature is built and waiting to go up
	if openChild {
		phase = "working"
	}
	if err := ps.SetState(store.AgentState{
		Agent: agent, Container: "td-EPIC", Branch: "td-EPIC", Task: "td-1", Phase: phase,
	}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	// Work on the branch for submit to record.
	if err := os.WriteFile(filepath.Join(root, ".worktrees", agent, "feature.txt"), []byte("built\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{Root: root}
	return newEngine(t, st, deps), ps, registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: phase}, deps
}
