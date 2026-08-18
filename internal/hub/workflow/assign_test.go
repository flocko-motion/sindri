package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestNextUpRanksPackagesAndLeavesTogether is the rule the two-queue assigner broke: a rating is
// worth the same wherever it sits. The reported case is the first row — a mid package went out
// while a critical task sat open, and no reading of the backlog explained why.
func TestNextUpRanksPackagesAndLeavesTogether(t *testing.T) {
	pkg := func(id, prio string) store.Task { return store.Task{ID: id, Priority: prio} }
	for _, c := range []struct {
		name             string
		packages, leaves []store.Task
		want             string
		wantPackage      bool
	}{
		{"a critical task beats a mid package",
			[]store.Task{pkg("td-pkg", "P2")}, []store.Task{pkg("td-crit", "P0")}, "td-crit", false},
		{"a critical package beats a mid task",
			[]store.Task{pkg("td-pkg", "P0")}, []store.Task{pkg("td-mid", "P2")}, "td-pkg", true},
		{"an equal rating falls to the id, and the leaf can win it",
			[]store.Task{pkg("td-z", "P1")}, []store.Task{pkg("td-a", "P1")}, "td-a", false},
		{"an equal rating falls to the id, and the package can win it",
			[]store.Task{pkg("td-a", "P1")}, []store.Task{pkg("td-z", "P1")}, "td-a", true},
		{"only packages", []store.Task{pkg("td-pkg", "P3")}, nil, "td-pkg", true},
		{"only leaves", nil, []store.Task{pkg("td-leaf", "P3")}, "td-leaf", false},
	} {
		got, isPkg, ok := nextUp(c.packages, c.leaves)
		if !ok {
			t.Errorf("%s: nothing picked", c.name)
			continue
		}
		if got.ID != c.want || isPkg != c.wantPackage {
			t.Errorf("%s: picked %s (package=%v), want %s (package=%v)", c.name, got.ID, isPkg, c.want, c.wantPackage)
		}
	}
	if _, _, ok := nextUp(nil, nil); ok {
		t.Error("an empty backlog must pick nothing")
	}
}

// TestAnIdleWorkerTakesTheCriticalTaskOverAMidPackage is the same rule through the real claim path,
// which is where it went wrong: claimNext tried the package pool first and returned on any hit, so
// the leaf pool — holding every standalone task, at any rating — was never reached while one
// claimable package existed.
func TestAnIdleWorkerTakesTheCriticalTaskOverAMidPackage(t *testing.T) {
	const agent = "dvalin"
	root, _ := newWorkRepo(t, agent, "td-seed")
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: ".worktrees/" + agent}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	for _, x := range []store.OwnedTask{
		{ID: "td-pkg", Title: "a mid package", Status: "open", Priority: "P2", Type: "epic"},
		{ID: "td-kid", Title: "its subtask", Status: "open", Priority: "P2"},
		{ID: "td-crit", Title: "a critical task", Status: "open", Priority: "P0"},
	} {
		if err := ps.PutOwnedTask(x); err != nil {
			t.Fatalf("seed %s: %v", x.ID, err)
		}
	}
	if err := ps.SetParent("td-kid", "td-pkg"); err != nil {
		t.Fatalf("set parent: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Phase: "idle"}); err != nil {
		t.Fatalf("set state: %v", err)
	}

	e := New(st, &stubDeps{root: root})
	dir, err := e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-crit") {
		t.Errorf("directive = %q, want the critical task", dir)
	}
	held, _ := ps.GetState(agent)
	if held.Task != "td-crit" {
		t.Errorf("state.Task = %q, want td-crit", held.Task)
	}
	if held.Container != "" {
		t.Errorf("state.Container = %q, want empty — the mid package must wait its turn", held.Container)
	}
}
