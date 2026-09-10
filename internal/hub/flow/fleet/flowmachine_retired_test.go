package fleet

import (
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestARetiredAgentIsNeverAssignedWork is oin's loop, restated for a dispatcher that assigns rather
// than advertises. The old failure was a sweep pushing "sd-… is ready for you" at a retired agent
// every thirty seconds, which answered "still retired, waiting quietly" each time; the guard was
// applied in one sweep and skipped in its sibling three lines away. It now sits in the map, ahead of
// anything that could hand work over, so there is only one place for it to be missing from.
func TestARetiredAgentIsNeverAssignedWork(t *testing.T) {
	const agent = "oin"
	root, _ := flowtest.WorkRepo(t, agent, "seed")
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	a := store.Agent{Name: agent, Role: "worker", Workspace: ".worktrees/" + agent, Retired: true}
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "sd-free", Title: "claimable", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Phase: worker.Idle}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{Root: root, Alive: true}
	e := newEngine(t, st, deps)
	if err := e.taskAct().SyncTasks("repo"); err != nil {
		t.Fatal(err)
	}

	e.Look("repo", agent)
	got, _ := ps.GetState(agent)
	if got.Task != "" {
		t.Errorf("a retired agent must be handed nothing, got %q", got.Task)
	}
	if got.Phase != worker.Retired {
		t.Errorf("state = %q, want %q", got.Phase, worker.Retired)
	}
	if len(deps.InjectedText) != 0 {
		t.Errorf("and told nothing about work it cannot take: %v", deps.InjectedText)
	}

	// Back in service, the same pass hands it the work — the guard must not silence a working agent.
	a.Retired = false
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	e.Look("repo", agent)
	if got, _ := ps.GetState(agent); got.Task != "sd-free" {
		t.Errorf("an un-retired worker is Assigned the claimable task, got %+v", got)
	}
}
