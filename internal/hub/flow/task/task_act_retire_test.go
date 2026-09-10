package task

import (
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"path/filepath"
	"strings"
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
	return newActWith2(t, st, &flowtest.Hub{Root: root, Alive: true}), ps,
		registry.Caller{Project: proj, Agent: "dvalin", Role: "worker", Phase: "idle"}
}

// TestUnretiredWorkerIsServedAgain: winding down is reversible, and the refusal goes with the flag —
// nothing about the agent or the backlog was consumed while it was set.
func TestUnretiredWorkerIsServedAgain(t *testing.T) {
	a, ps, c := retireFixture(t)
	flowtest.Retire(t, ps, "dvalin")
	var out strings.Builder
	if _, err := a.CmdNext(c, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "retired") {
		t.Fatalf("setup: expected the retirement refusal, got %q", out.String())
	}

	ag, _, _ := ps.GetAgent("dvalin")
	ag.Retired = false
	if err := ps.PutAgent(ag); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if _, err := a.CmdNext(c, nil, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "retired") {
		t.Errorf("back in service, it must not still be refused: %q", out.String())
	}
}

// TestRetiringLeavesWorkInHand: the use case is "stop it once it is done", so what it holds stays
// with it. Taking the task back would be the opposite — interrupting the agent being wound down.
func TestRetiringLeavesWorkInHand(t *testing.T) {
	_, ps, _ := retireFixture(t)
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-1", Branch: "td-1", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	flowtest.Retire(t, ps, "dvalin")
	st, _ := ps.GetState("dvalin")
	if st.Task != "td-1" || st.Phase != "working" {
		t.Errorf("retiring must not disturb work in hand, got {task:%q phase:%q}", st.Task, st.Phase)
	}
}
