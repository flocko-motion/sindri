// package: hub/flow/task / idleworker_fixture_test
// type:    logic (an idle worker with something claimable waiting)
// job:     seed the shape a hand-over starts from — an agent at rest, a task open in the backlog,
// a worktree on disk — so a test about PREPARING a session has something to prepare it for.
// limits:  seeding only.
package task

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/store"
)

// idleWorkerWithOpenTask seeds a repo with one open, approved, prioritized leaf task and an idle
// worker with a worktree ready to claim it — ClaimNext's happy path, before any fullness gate.
func idleWorkerWithOpenTask(t *testing.T, deps *flowtest.Hub) (*Act, *store.ProjectStore) {
	t.Helper()
	const agent = "dvalin"
	root, _ := flowtest.WorkRepo(t, agent, "seed")
	deps.Root = root
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject(proj, root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For(proj)
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: ".worktrees/" + agent}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-abc123", Title: "a task", Status: "open", Priority: "P2"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	// Both rows, stated: the owned row is the source and the cached one is what every claim reads.
	// Written here rather than synced, because the sync belongs to flow/task — which imports this
	// package, so a fixture reaching for it would be a cycle.
	if err := ps.UpsertTask(store.Task{ID: "td-abc123", Title: "a task", Status: "open", Priority: "P2"}); err != nil {
		t.Fatalf("seed cache row: %v", err)
	}
	a := newActWith2(t, st, deps)
	return a, ps
}
