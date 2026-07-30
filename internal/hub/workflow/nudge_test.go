package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// nudgeStore sets up a project with one idle worker plus the agents that must NOT be nudged.
func nudgeStore(t *testing.T) (*Engine, *stubDeps, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("proj")
	for _, a := range []store.Agent{
		{Name: "nori", Role: "worker", Workspace: ".worktrees/nori"},
		{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"},
		{Name: "galar", Role: "planner", Workspace: ".worktrees/galar"},
		{Name: "hepti", Role: "coauthor", Workspace: "."},
	} {
		if err := ps.PutAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	// dvalin is busy, galar plans, hepti collaborates — only nori is free.
	_ = ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-busy", Branch: "td-busy", Phase: "working"})
	_ = ps.SetState(store.AgentState{Agent: "galar", Phase: "planning"})
	_ = ps.SetState(store.AgentState{Agent: "hepti", Phase: "collab"})
	_ = ps.SetState(store.AgentState{Agent: "nori", Phase: "idle"})

	deps := &stubDeps{root: t.TempDir(), alive: true}
	return New(st, deps), deps, ps
}

// TestNudgeReachesOnlyTheIdleWorker: Notify wakes an agent already blocked asking for work, so an
// agent that asked, got nothing and stopped asking never hears about a new task — it sits idle
// beside a claimable one indefinitely. The nudge is what closes that, and it must not disturb
// agents holding work or roles that never claim backlog tasks.
func TestNudgeReachesOnlyTheIdleWorker(t *testing.T) {
	e, deps, _ := nudgeStore(t)
	e.nudgeIdleWorkers("proj", "td-ae5ca0", "P2")

	if len(deps.injected) != 1 || deps.injected[0] != "nori" {
		t.Fatalf("only the idle worker should be nudged, got %v", deps.injected)
	}
}

// TestNudgeSkipsUnratedWork: a worker won't claim an unrated task, so nudging for one would send it
// to fetch work it has to refuse — noise that teaches the agent the nudge means nothing.
func TestNudgeSkipsUnratedWork(t *testing.T) {
	e, deps, _ := nudgeStore(t)
	e.nudgeIdleWorkers("proj", "gh-9", "")

	if len(deps.injected) != 0 {
		t.Fatalf("an unrated task must not nudge anyone, got %v", deps.injected)
	}
}

// TestNudgeSkipsADeadAgent: injecting into a container that isn't running writes nowhere.
func TestNudgeSkipsADeadAgent(t *testing.T) {
	e, deps, _ := nudgeStore(t)
	deps.alive = false
	e.nudgeIdleWorkers("proj", "td-ae5ca0", "P2")

	if len(deps.injected) != 0 {
		t.Fatalf("a dead agent cannot be nudged, got %v", deps.injected)
	}
}

// TestWorkAvailableSendsThemToTheClaimPath: the message must point at `sindri`, not name the task as
// theirs. Claiming stays a pull through one path, so two idle workers can't take the same task, and
// the one that asks may rightly get a higher-priority task instead.
func TestWorkAvailableSendsThemToTheClaimPath(t *testing.T) {
	msg := MsgWorkAvailable("td-ae5ca0")
	if !strings.Contains(msg, "td-ae5ca0") {
		t.Errorf("the nudge should name the work that arrived: %q", msg)
	}
	if !strings.Contains(msg, "`sindri`") {
		t.Errorf("the nudge must send the agent through the claim path: %q", msg)
	}
}
