package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// nudgeStore sets up a project with one idle worker plus the agents that must NOT be nudged, and
// one open, rated, claimable task — nudgeIdleWorkers now asks the real backlog, not a bare id.
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
	if err := ps.UpsertTask(store.Task{ID: "td-ae5ca0", Title: "fix it", Status: "open", Priority: "P2"}); err != nil {
		t.Fatal(err)
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
	e.nudgeIdleWorkers("proj", "P2")

	if len(deps.injected) != 1 || deps.injected[0] != "nori" {
		t.Fatalf("only the idle worker should be nudged, got %v", deps.injected)
	}
}

// TestNudgeSkipsUnratedWork: an unrated creation cannot itself have made anything newly claimable,
// so it is not worth a scan.
func TestNudgeSkipsUnratedWork(t *testing.T) {
	e, deps, _ := nudgeStore(t)
	e.nudgeIdleWorkers("proj", "")

	if len(deps.injected) != 0 {
		t.Fatalf("an unrated trigger must not nudge anyone, got %v", deps.injected)
	}
}

// TestNudgeSkipsADeadAgent: injecting into a container that isn't running writes nowhere.
func TestNudgeSkipsADeadAgent(t *testing.T) {
	e, deps, _ := nudgeStore(t)
	deps.alive = false
	e.nudgeIdleWorkers("proj", "P2")

	if len(deps.injected) != 0 {
		t.Fatalf("a dead agent cannot be nudged, got %v", deps.injected)
	}
}

// TestNudgeCapsAtHowManyTasksAreClaimable is sd-4589ef's herd fix: two idle workers behind one
// claimable task must wake only one of them, not both to race for it.
func TestNudgeCapsAtHowManyTasksAreClaimable(t *testing.T) {
	e, deps, ps := nudgeStore(t)
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Phase: "idle"}); err != nil {
		t.Fatal(err) // dvalin is free too now — still only one task to give out
	}
	e.nudgeIdleWorkers("proj", "P2")

	if len(deps.injected) != 1 {
		t.Fatalf("one task should wake exactly one worker, got %v", deps.injected)
	}
}

// TestNudgeSkipsAnAgentExplainNextWouldRuleOut is the predicate the ticket asks for: retired,
// clear-armed and a full context all say "takes nothing regardless of the backlog" (-> agentBlocked),
// so nudging past them wastes a turn on an agent that was always going to be told no.
func TestNudgeSkipsAnAgentExplainNextWouldRuleOut(t *testing.T) {
	for _, mutate := range []struct {
		name string
		do   func(ps *store.ProjectStore)
	}{
		{"retired", func(ps *store.ProjectStore) {
			a, _, _ := ps.GetAgent("nori")
			a.Retired = true
			_ = ps.PutAgent(a)
		}},
		{"clear-armed", func(ps *store.ProjectStore) {
			a, _, _ := ps.GetAgent("nori")
			a.ClearArmed = true
			_ = ps.PutAgent(a)
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			e, deps, ps := nudgeStore(t)
			mutate.do(ps)
			e.nudgeIdleWorkers("proj", "P2")
			if len(deps.injected) != 0 {
				t.Errorf("a %s worker must not be nudged, got %v", mutate.name, deps.injected)
			}
		})
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
