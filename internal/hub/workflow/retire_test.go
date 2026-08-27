package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
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
	return New(st, &stubDeps{root: root, alive: true}), ps,
		registry.Caller{Project: "proj", Agent: "dvalin", Role: "worker", Phase: "idle"}
}

// retire sets the flag the way the hub's service does.
func retire(t *testing.T, ps *store.ProjectStore, name string) {
	t.Helper()
	a, _, _ := ps.GetAgent(name)
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
}

// TestRetiredWorkerIsHandedNothing is the point of the flag: an agent the user is winding down takes
// no further work, however much is waiting. The gate sits in claimNext so it holds whichever way the
// work would have arrived — the agent asking, or the hub waking it.
func TestRetiredWorkerIsHandedNothing(t *testing.T) {
	e, ps, c := retireFixture(t)
	retire(t, ps, "dvalin")

	if _, claimed, err := e.claimNext("proj", "dvalin"); err != nil || claimed {
		t.Fatalf("a retired worker must claim nothing: claimed=%v err=%v", claimed, err)
	}
	// The task is untouched and still open for somebody else.
	if task, _, _ := ps.GetTask("td-1"); task.Status != "open" {
		t.Errorf("the task should stay open for another worker, got %q", task.Status)
	}
	// And it is TOLD, rather than left blocking on a queue it is no longer served from.
	var out strings.Builder
	if code, err := e.CmdNext(c, nil, &out); err != nil || code != 0 {
		t.Fatalf("CmdNext: code=%d err=%v", code, err)
	}
	if !strings.Contains(out.String(), "retired") {
		t.Errorf("a retired worker should be told why it gets nothing: %q", out.String())
	}
	// The blocking path answers at once too — waiting forever is what "no tasks" would have meant.
	d, err := e.AgentDirective(context.Background(), "proj", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(d, "retired") {
		t.Errorf("the directive should say it is retired, got %q", d)
	}
}

// TestUnretiredWorkerIsServedAgain: winding down is reversible, and the refusal goes with the flag —
// nothing about the agent or the backlog was consumed while it was set.
func TestUnretiredWorkerIsServedAgain(t *testing.T) {
	e, ps, c := retireFixture(t)
	retire(t, ps, "dvalin")
	var out strings.Builder
	if _, err := e.CmdNext(c, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "retired") {
		t.Fatalf("setup: expected the retirement refusal, got %q", out.String())
	}

	a, _, _ := ps.GetAgent("dvalin")
	a.Retired = false
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if _, err := e.CmdNext(c, nil, &out); err != nil {
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
	retire(t, ps, "dvalin")
	st, _ := ps.GetState("dvalin")
	if st.Task != "td-1" || st.Phase != "working" {
		t.Errorf("retiring must not disturb work in hand, got {task:%q phase:%q}", st.Task, st.Phase)
	}
}
