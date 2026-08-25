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

// TestNudgeIdleWorkersDoesNotRepeatIdenticalNudges: notifyOnce's dedup is shared with AssignPendingWork
// — a second creation event finding the same open task and idle worker must not push about it twice.
func TestNudgeIdleWorkersDoesNotRepeatIdenticalNudges(t *testing.T) {
	e, deps, _ := nudgeStore(t)
	e.nudgeIdleWorkers("proj", "P2")
	e.nudgeIdleWorkers("proj", "P2")

	if len(deps.injected) != 1 {
		t.Fatalf("the same unclaimed task must be pushed once, not on every creation event, got %v", deps.injected)
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

// TestAssignPendingWorkSkipsARetiredOrEscalatedAgent pins sd-72af71: AssignPendingWork is a second,
// independent push path from nudgeIdleWorkers (the periodic sweep, not the event-triggered one), and
// it forgot the same exemption — the bug that woke a retired agent every sweep and then refused it.
// idleWorkerWithOpenTask, not nudgeStore: AssignPendingWork calls SyncTasks first, which a plain
// UpsertTask (nudgeStore's fixture) does not survive — only an owned task does.
func TestAssignPendingWorkSkipsARetiredOrEscalatedAgent(t *testing.T) {
	for _, mutate := range []struct {
		name string
		do   func(ps *store.ProjectStore)
	}{
		{"retired", func(ps *store.ProjectStore) {
			a, _, _ := ps.GetAgent("dvalin")
			a.Retired = true
			_ = ps.PutAgent(a)
		}},
		{"clear-armed", func(ps *store.ProjectStore) {
			a, _, _ := ps.GetAgent("dvalin")
			a.ClearArmed = true
			_ = ps.PutAgent(a)
		}},
		{"escalated", func(ps *store.ProjectStore) {
			_ = ps.SetEscalation("dvalin", "one column or two?")
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			deps := &stubDeps{}
			e, ps := idleWorkerWithOpenTask(t, deps)
			mutate.do(ps)
			e.AssignPendingWork("repo")
			if len(deps.injected) != 0 {
				t.Errorf("a %s worker must not be pushed, got %v", mutate.name, deps.injected)
			}
		})
	}
}

// TestAssignPendingWorkDoesNotRepeatIdenticalNudges pins the second half of sd-72af71: a sweep that
// finds the same task still unclaimed must not push about it again — the fortieth identical wake is
// noise that teaches an agent to stop reading the channel.
func TestAssignPendingWorkDoesNotRepeatIdenticalNudges(t *testing.T) {
	deps := &stubDeps{}
	e, _ := idleWorkerWithOpenTask(t, deps)
	e.AssignPendingWork("repo")
	e.AssignPendingWork("repo")

	if len(deps.injected) != 1 {
		t.Fatalf("the same unclaimed task must be pushed once, not on every sweep, got %v", deps.injected)
	}
}

// TestAssignPendingWorkNudgesAgainWhenTheTaskChanges: the dedup remembers what an agent was last told,
// not that it was told SOMETHING — a new claimable task still deserves its own push.
func TestAssignPendingWorkNudgesAgainWhenTheTaskChanges(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	e.AssignPendingWork("repo")
	if len(deps.injected) != 1 {
		t.Fatalf("setup: expected the first nudge, got %v", deps.injected)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-def456", Title: "also fix it", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	e.AssignPendingWork("repo")
	if len(deps.injected) != 2 {
		t.Fatalf("a new claimable task should still be pushed, got %v", deps.injected)
	}
}

// TestAssignPendingWorkReoffersAfterReopening pins Finding 3 (review round 3): last_nudge must not
// outlive the offer it recorded — a task nudged, then closed, then reopened under the same id must be
// announced again, not silently skipped for ever.
func TestAssignPendingWorkReoffersAfterReopening(t *testing.T) {
	deps := &stubDeps{}
	e, _ := idleWorkerWithOpenTask(t, deps)
	e.AssignPendingWork("repo")
	if len(deps.injected) != 1 {
		t.Fatalf("setup: expected the first nudge, got %v", deps.injected)
	}

	if err := e.SetStatus("repo", "td-abc123", "closed"); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshTask("repo", "td-abc123"); err != nil {
		t.Fatal(err)
	}
	e.AssignPendingWork("repo") // a sweep while it is genuinely closed — this must forget the memory

	if err := e.SetStatus("repo", "td-abc123", "open"); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshTask("repo", "td-abc123"); err != nil {
		t.Fatal(err)
	}
	e.AssignPendingWork("repo")

	if len(deps.injected) != 2 {
		t.Fatalf("a task closed then reopened under the same id must be announced again, got %v", deps.injected)
	}
}

// TestAssignPendingSubtaskReoffersAfterReopening is the container-path analogue: assignPendingSubtask
// forgot to forget too (review round 4), so a subtask closed then reopened under the same id was
// never announced again on that path.
func TestAssignPendingSubtaskReoffersAfterReopening(t *testing.T) {
	root, _ := newWorkRepo(t, "dain", "seed")
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "dain", Role: "worker", Workspace: ".worktrees/dain"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-EPIC", Title: "a feature", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-1", Title: "a subtask", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetParent("td-1", "td-EPIC"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{}
	e := New(st, deps)

	e.AssignPendingWork("repo")
	if len(deps.injected) != 1 {
		t.Fatalf("setup: expected the first nudge, got %v", deps.injected)
	}

	if err := e.SetStatus("repo", "td-1", "closed"); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshTask("repo", "td-1"); err != nil {
		t.Fatal(err)
	}
	e.AssignPendingWork("repo") // genuinely gone — this must forget the memory

	if err := e.SetStatus("repo", "td-1", "open"); err != nil {
		t.Fatal(err)
	}
	if err := e.RefreshTask("repo", "td-1"); err != nil {
		t.Fatal(err)
	}
	e.AssignPendingWork("repo")

	if len(deps.injected) != 2 {
		t.Fatalf("a subtask closed then reopened under the same id must be announced again, got %v", deps.injected)
	}
}

// TestReviewDirectiveRefusesARetiredReviewer: the non-blocking finding from review round 4 —
// reviewDirective had no retired check, so a retired reviewer could still be handed a fresh claim.
func TestReviewDirectiveRefusesARetiredReviewer(t *testing.T) {
	st, ps := poolFixture(t)
	if err := ps.PutAgent(store.Agent{Name: "fili", Role: "reviewer", Workspace: ".worktrees/fili", Retired: true}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: t.TempDir(), alive: true})

	dir, _, err := e.reviewDirective("repo", "fili")
	if err != nil {
		t.Fatal(err)
	}
	if dir != DirRetired {
		t.Errorf("directive = %q, want DirRetired", dir)
	}
	if held, _ := ps.ReviewingPR("fili"); held != "" {
		t.Errorf("a retired reviewer must not claim anything, got %q", held)
	}
}

// TestWakeRefusalDoesNotBlockAnAgentHoldingWork pins Finding 1 (review round 3): retired and
// clear-armed only refuse the idle path directive() falls to — an agent holding a task gets its real
// directive (a rejection, a run result, a merge conflict) regardless of either state.
func TestWakeRefusalDoesNotBlockAnAgentHoldingWork(t *testing.T) {
	for _, mutate := range []struct {
		name string
		do   func(a *store.Agent)
	}{
		{"retired", func(a *store.Agent) { a.Retired = true }},
		{"clear-armed", func(a *store.Agent) { a.ClearArmed = true }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { st.Close() })
			ps := st.For("repo")
			a := store.Agent{Name: "dvalin", Role: "worker"}
			mutate.do(&a)
			if err := ps.PutAgent(a); err != nil {
				t.Fatal(err)
			}
			if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-1", Branch: "td-1", Phase: "submitted"}); err != nil {
				t.Fatal(err)
			}
			e := New(st, &stubDeps{root: t.TempDir()})

			if r := e.WakeRefusal("repo", "dvalin"); r != "" {
				t.Errorf("an agent holding a task must not be refused a wake, got %q", r)
			}
		})
	}
}

// TestWakeRefusalIgnoresRetiredBetweenSubtasks pins review round 4's finding: a retired feature
// worker between subtasks still gets its next one from claimNextSubtask, which gates only on
// clearArmed — never on retired.
func TestWakeRefusalIgnoresRetiredBetweenSubtasks(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "dain", Role: "worker", Retired: true}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-EPIC", Title: "a feature", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: t.TempDir()})

	if r := e.WakeRefusal("repo", "dain"); r != "" {
		t.Errorf("a retired feature worker between subtasks must not be refused, got %q", r)
	}
}

// TestWakeRefusalStillGatesClearArmedBetweenSubtasks is the converse: claimNextSubtask DOES check
// clearArmed, so that state must still refuse the same shape of agent.
func TestWakeRefusalStillGatesClearArmedBetweenSubtasks(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "dain", Role: "worker", ClearArmed: true}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-EPIC", Title: "a feature", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: t.TempDir()})

	if r := e.WakeRefusal("repo", "dain"); r == "" {
		t.Error("a clear-armed feature worker between subtasks must still be refused")
	}
}

// TestWakeRefusalIgnoresRetiredAwaitingItsOwnPR pins the other gap: directive()'s idle branch checks
// AwaitingPR before the retired check, so that verdict reaches the agent regardless.
func TestWakeRefusalIgnoresRetiredAwaitingItsOwnPR(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Retired: true}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-td-1", Task: "td-1", Agent: "dvalin", Branch: "td-1", Base: "main", Status: "rejected"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, &stubDeps{root: t.TempDir()})

	if r := e.WakeRefusal("repo", "dvalin"); r != "" {
		t.Errorf("an agent awaiting its own PR must not be refused, got %q", r)
	}
}

// TestWakeRefusalNeverBlocksAPlannerOrCoauthor: directive() never checks retired/clear-armed for
// either role, so WakeRefusal must not invent a refusal neither would ever actually answer with.
func TestWakeRefusalNeverBlocksAPlannerOrCoauthor(t *testing.T) {
	for _, role := range []string{"planner", "coauthor"} {
		t.Run(role, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { st.Close() })
			ps := st.For("repo")
			if err := ps.PutAgent(store.Agent{Name: "galar", Role: role, Retired: true}); err != nil {
				t.Fatal(err)
			}
			if err := ps.SetState(store.AgentState{Agent: "galar", Phase: "idle"}); err != nil {
				t.Fatal(err)
			}
			e := New(st, &stubDeps{root: t.TempDir()})

			if r := e.WakeRefusal("repo", "galar"); r != "" {
				t.Errorf("a %s is never refused this way, got %q", role, r)
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
