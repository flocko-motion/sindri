package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// unownedSubtask seeds a feature sindri owns whose remaining subtask is an openspec change it does
// not — a planner hanging a spec under a feature, which is how the tree in the field was shaped.
func unownedSubtask(t *testing.T) (*Engine, *store.ProjectStore) {
	t.Helper()
	const agent = "dvalin"
	root, _ := newWorkRepo(t, agent, "td-feat")
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-feat", Title: "a feature", Status: "open", Priority: "P1", Type: "epic"}); err != nil {
		t.Fatalf("own the feature: %v", err)
	}
	for _, x := range []struct{ id, parent, status string }{
		{"td-feat", "", "open"}, {"os-spec", "td-feat", "open"},
	} {
		if err := ps.UpsertTask(store.Task{
			ID: x.id, Title: x.id, Status: x.status, ParentID: x.parent, Priority: "P1",
		}); err != nil {
			t.Fatalf("seed %s: %v", x.id, err)
		}
	}
	if err := ps.SetParent("os-spec", "td-feat"); err != nil {
		t.Fatalf("parent os-spec: %v", err)
	}
	// Holding the feature, between subtasks — the state the directive and the submit gate disagreed in.
	if err := ps.SetState(store.AgentState{
		Agent: agent, Container: "td-feat", Branch: "td-feat", Phase: "idle",
	}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	return newEngine(st, &stubDeps{root: root}), ps
}

// TestTheDirectiveAndTheSubmitGateAgree is the loop a worker reported from inside a feature: the
// directive said the feature was finished, submit refused it for an open subtask, and checkpoint had
// nothing to checkpoint. One swallowed error caused all three — assignment of an os- subtask writes
// owned_tasks, which fails for an id sindri does not own, and that failure was returned as "no work
// left". Both paths ask OpenSubtasks the same question, so they must never answer differently.
func TestTheDirectiveAndTheSubmitGateAgree(t *testing.T) {
	e, ps := unownedSubtask(t)

	open, err := ps.OpenSubtasks("td-feat")
	if err != nil {
		t.Fatalf("OpenSubtasks: %v", err)
	}
	if len(open) != 1 || open[0].ID != "os-spec" {
		t.Fatalf("the feature has one subtask left, got %d", len(open))
	}

	// The directive must hand that subtask over, not report the feature done.
	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "os-spec") {
		t.Errorf("the directive should hand over the open subtask, got: %q", dir)
	}
	if !strings.Contains(dir, "`sindri checkpoint") {
		t.Errorf("the subtask is ended by a checkpoint, which the directive must name: %q", dir)
	}
	if strings.Contains(dir, "is finished") {
		t.Errorf("a feature with an open subtask is not finished: %q", dir)
	}

	// And the agent is now ON it — the assignment landed in agent_state, since an os- task carries
	// no status of sindri's to set.
	st, _ := ps.GetState("dvalin")
	if st.Task != "os-spec" || st.Phase != "working" {
		t.Fatalf("state after the directive = {task:%q phase:%q}, want {os-spec working}", st.Task, st.Phase)
	}
	if st.Container != "td-feat" {
		t.Errorf("the feature must still be held, got %q", st.Container)
	}
}

// TestAdvanceSurfacesAFailureInsteadOfReportingDone: whatever else goes wrong, "no work left" must
// mean the store had none — never that assigning it failed. That conflation is what let the hub
// contradict itself across three commands.
func TestAdvanceSurfacesAFailureInsteadOfReportingDone(t *testing.T) {
	e, ps := unownedSubtask(t)
	next, ok, err := e.advanceContainer("repo", "dvalin", "td-feat")
	if err != nil || !ok {
		t.Fatalf("advanceContainer over an unowned subtask: ok=%v err=%v", ok, err)
	}
	if next.ID != "os-spec" {
		t.Errorf("next = %q, want os-spec", next.ID)
	}
	// With it done, the feature really is empty — and that is the only thing "not ok" may mean.
	if err := ps.UpsertTask(store.Task{ID: "os-spec", Title: "os-spec", Status: "closed", ParentID: "td-feat"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := e.advanceContainer("repo", "dvalin", "td-feat"); ok || err != nil {
		t.Errorf("an empty feature should report no work and no error, got ok=%v err=%v", ok, err)
	}
}
