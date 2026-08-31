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
		got, isPkg, ok := nextUp(c.packages, c.leaves, nil)
		if !ok {
			t.Errorf("%s: nothing picked", c.name)
			continue
		}
		if got.ID != c.want || isPkg != c.wantPackage {
			t.Errorf("%s: picked %s (package=%v), want %s (package=%v)", c.name, got.ID, isPkg, c.want, c.wantPackage)
		}
	}
	if _, _, ok := nextUp(nil, nil, nil); ok {
		t.Error("an empty backlog must pick nothing")
	}
}

// TestNextUpTiebreaksTowardThePreferredTaskWithinPriorityOnly: prefers may pick a lower-id task
// among those tied on the best priority present, but must never reach past a higher-priority one —
// priority is the user's flow control, not the fleet's to spend for its own convenience.
func TestNextUpTiebreaksTowardThePreferredTaskWithinPriorityOnly(t *testing.T) {
	pkg := func(id, prio string) store.Task { return store.Task{ID: id, Priority: prio} }
	preferZ := func(t store.Task) bool { return t.ID == "td-z" }

	// Two leaves tied on P1: without a preference the lower id wins; with one preferring td-z, it does.
	leaves := []store.Task{pkg("td-a", "P1"), pkg("td-z", "P1")}
	if got, _, ok := nextUp(nil, leaves, nil); !ok || got.ID != "td-a" {
		t.Fatalf("no preference: got %q, want td-a (lower id)", got.ID)
	}
	if got, _, ok := nextUp(nil, leaves, preferZ); !ok || got.ID != "td-z" {
		t.Fatalf("preferring td-z among equals: got %q, want td-z", got.ID)
	}

	// A higher-priority task the preference does NOT name must still win outright.
	mixed := []store.Task{pkg("td-crit", "P0"), pkg("td-z", "P1")}
	if got, _, ok := nextUp(nil, mixed, preferZ); !ok || got.ID != "td-crit" {
		t.Fatalf("a preference must never reach past a higher priority: got %q, want td-crit", got.ID)
	}

	// The same rule ACROSS the two pools: each pool's own top slice is computed separately, so a
	// preferred package at ITS pool's best priority must not beat a higher-priority leaf in the
	// other pool, even though neither pool's slice alone shows the difference.
	crossPackages := []store.Task{pkg("td-pkg", "P3")}
	crossLeaves := []store.Task{pkg("td-crit2", "P0")}
	preferPkg := func(t store.Task) bool { return t.ID == "td-pkg" }
	if got, isPkg, ok := nextUp(crossPackages, crossLeaves, preferPkg); !ok || got.ID != "td-crit2" || isPkg {
		t.Fatalf("a preference must never reach past a higher priority in the OTHER pool: got %q (package=%v), want td-crit2", got.ID, isPkg)
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
		// Both untiered, defaulting to mid (-> api.TierOrDefault), which the stub below maps onto
		// the model the worker is already running — so the preference is LIVE for both, not inert,
		// which is what let the cross-pool bug through undetected: the old code returned on the
		// FIRST hit in the package pool without ever comparing priority against the leaf pool.
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
	if err := ps.SetState(store.AgentState{Agent: agent, Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}

	e := New(st, &stubDeps{
		root:         root,
		currentModel: "claude-sonnet-5",
		tierModels:   map[string]string{"mid": "claude-sonnet-5"},
	})
	// claimNext reads the synced cache; warm it now so the assertions below see this seeding, not
	// whatever the cache held (nothing) before it.
	if err := e.SyncTasks("repo"); err != nil {
		t.Fatalf("sync tasks: %v", err)
	}
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

// TestAMismatchedTaskChangesTheModelThenHandsItOver: the model change queues its switch into the
// live session (-> agent.Service.SetModel) — no relaunch — with the claimed directive as SetModel's
// own next, queued behind it. This ask still answers DirPreparing rather than the directive
// directly: the switch clears first, and handing the agent something to act on right before that
// would be exactly the cut-off compaction's own fix avoids.
func TestAMismatchedTaskChangesTheModelThenHandsItOver(t *testing.T) {
	deps := &stubDeps{
		alive:        true,
		currentModel: "claude-haiku-4-5",
		tierModels:   map[string]string{"senior": "claude-opus-5"},
	}
	e, ps := idleWorkerWithOpenTask(t, deps)
	if err := ps.SetOwnedTier("td-abc123", "senior"); err != nil {
		t.Fatal(err)
	}
	e.refreshCachedTask("repo", "td-abc123")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the model switch is about to clear the session", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want the mismatched task claimed", st.Task)
	}
	if len(deps.modelSet) != 1 || deps.modelSet[0] != "dvalin=claude-opus-5" {
		t.Errorf("modelSet = %v, want exactly one SetModel(dvalin, claude-opus-5)", deps.modelSet)
	}
	if len(deps.injectedText) != 1 || !strings.Contains(deps.injectedText[0], "td-abc123") {
		t.Errorf("injectedText = %v, want the claimed directive delivered once the switch answered", deps.injectedText)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — a model change clears rather than compacts", deps.compacted)
	}
}

// TestAMatchingTaskIsHandedOverWithoutChangingTheModel is the control: nothing about the tier check
// should stop an ordinary claim when the model already matches.
func TestAMatchingTaskIsHandedOverWithoutChangingTheModel(t *testing.T) {
	deps := &stubDeps{
		alive:        true,
		currentModel: "claude-opus-5",
		tierModels:   map[string]string{"senior": "claude-opus-5"},
	}
	e, ps := idleWorkerWithOpenTask(t, deps)
	if err := ps.SetOwnedTier("td-abc123", "senior"); err != nil {
		t.Fatal(err)
	}
	e.refreshCachedTask("repo", "td-abc123")

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-abc123") {
		t.Errorf("directive = %q, want the task claimed — the model already matches", dir)
	}
	if len(deps.modelSet) != 0 {
		t.Errorf("modelSet = %v, want none — the model already matches", deps.modelSet)
	}
}
