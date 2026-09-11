package task

import (
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// retireFixture is one worker and one open, claimable task.
func retireFixture(t *testing.T) (*Act, *store.ProjectStore, registry.Caller) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject(proj, root); err != nil {
		t.Fatal(err)
	}
	ps := st.For(proj)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "work", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	return newActWith2(t, st, &flowtest.Hub{Root: root}), ps,
		registry.Caller{Project: proj, Agent: "dvalin", Role: "worker", Phase: "idle"}
}

// TestRetiringLeavesWorkInHand: the use case is "stop it once it is done", so what it holds stays
// with it. Taking the task back would be the opposite — interrupting the agent being wound down.
func TestRetiringLeavesWorkInHand(t *testing.T) {
	_, ps, _ := retireFixture(t)
	flowtest.Place(t, ps, store.AgentState{Agent: "dvalin", Task: "td-1", Branch: "td-1", Phase: "working"})
	flowtest.Retire(t, ps, "dvalin")
	st, _ := ps.GetState("dvalin")
	if st.Task != "td-1" || st.Phase != worker.Working {
		t.Errorf("retiring must not disturb work in hand, got {task:%q phase:%q}", st.Task, st.Phase)
	}
}
