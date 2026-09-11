package agent

import (
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// stallStore is one worker holding a task, and one waiting on a verdict.
func stallStore(t *testing.T) (*Act, *flowtest.Hub, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For(proj)
	for _, a := range []store.Agent{
		{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"},
		{Name: "nori", Role: "worker", Workspace: ".worktrees/nori"},
	} {
		if err := ps.PutAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	flowtest.Place(t, ps, store.AgentState{Agent: "dvalin", Task: "td-d9a8c3", Branch: "td-d9a8c3", Phase: "working"})
	flowtest.Place(t, ps, store.AgentState{Agent: "nori", Task: "td-other", Branch: "td-other", Phase: "submitted"})
	deps := &flowtest.Hub{Root: t.TempDir()}
	return newActOn(t, st, deps), deps, ps
}

// TestNudgeStalledNamesTheTaskAndTheTwoWaysOut: a stalled agent has lost the thread, so the prod
// names the task it still holds and the only two endings it has — finish, or escalate. It used to
// invite "say what blocks you", which agents took as licence to narrate at the user and wait.
func TestNudgeStalledNamesTheTaskAndTheTwoWaysOut(t *testing.T) {
	a, deps, _ := stallStore(t)

	if !a.NudgeStalled(proj, "dvalin", flowtest.Saying("idle"), StallDwell+time.Minute) {
		t.Fatal("a worker holding work and gone quiet should be nudged")
	}
	if len(deps.Injected) != 1 || deps.Injected[0] != "dvalin" {
		t.Fatalf("expected dvalin to be nudged, got %v", deps.Injected)
	}
	got := deps.InjectedText[0]
	for _, want := range []string{"td-d9a8c3", "no third", "sindri escalate"} {
		if !strings.Contains(got, want) {
			t.Errorf("the nudge should mention %q, got:\n%s", want, got)
		}
	}
}

// reviewingAgent adds a reviewer holding an Assigned review of pr to a stallStore, in the state
// AssignReview leaves it in: the phase, and no Task or Container of any kind.
func reviewingAgent(t *testing.T, ps *store.ProjectStore, name, pr string) {
	t.Helper()
	if err := ps.PutAgent(store.Agent{Name: name, Role: "reviewer", Workspace: ".worktrees/" + name}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: pr, Task: "td-d9a8c3", Agent: "dvalin", Branch: pr, Status: "open"}); err != nil {
		t.Fatal(err)
	}
	id, err := ps.AddReview(pr, "review it")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AssignReview(id, name); err != nil {
		t.Fatal(err)
	}
	flowtest.Place(t, ps, store.AgentState{Agent: name, Phase: "reviewing"})
}

// TestAStalledReviewerIsNudgedAboutItsPR (sd-9a7f80): a reviewer's hold lives in the review row, so
// the nudge had nothing to name and bailed. Both halves must land — Stalled counting "reviewing" is
// inert while this returns false — so this asserts the whole path, not the rule alone.
func TestAStalledReviewerIsNudgedAboutItsPR(t *testing.T) {
	a, deps, ps := stallStore(t)
	reviewingAgent(t, ps, "ori", "pr-42")

	if !a.NudgeStalled(proj, "ori", flowtest.Saying("idle"), StallDwell+time.Minute) {
		t.Fatal("a reviewer holding a PR and gone quiet should be nudged")
	}
	if len(deps.Injected) != 1 || deps.Injected[0] != "ori" {
		t.Fatalf("expected ori to be nudged, got %v", deps.Injected)
	}
	if got := deps.InjectedText[0]; !strings.Contains(got, "pr-42") {
		t.Errorf("the nudge must name the PR it holds, got:\n%s", got)
	}
	events, err := ps.Events("ori", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Type == "nudge" && strings.Contains(ev.Payload, "pr-42") {
			return
		}
	}
	t.Errorf("expected a logged nudge naming the PR, got %+v", events)
}

// TestAReviewerWithNothingToNameIsLeftAlone: the fallback reads the review row rather than assuming
// one, so a phase left behind by a released review produces silence instead of an invented id.
func TestAReviewerWithNothingToNameIsLeftAlone(t *testing.T) {
	a, deps, ps := stallStore(t)
	if err := ps.PutAgent(store.Agent{Name: "ori", Role: "reviewer", Workspace: ".worktrees/ori"}); err != nil {
		t.Fatal(err)
	}
	flowtest.Place(t, ps, store.AgentState{Agent: "ori", Phase: "reviewing"})
	if a.NudgeStalled(proj, "ori", flowtest.Saying("idle"), StallDwell+time.Minute) {
		t.Error("with no review held there is nothing to say, so nothing should be sent")
	}
	if len(deps.Injected) != 0 {
		t.Errorf("nothing should have been injected, got %v", deps.Injected)
	}
}

// TestNudgeStalledLeavesWaitingAgentsAlone: "submitted" is waiting for a verdict the agent cannot
// hurry, so prodding it would be telling it off for doing the right thing.
func TestNudgeStalledLeavesWaitingAgentsAlone(t *testing.T) {
	a, deps, _ := stallStore(t)
	if a.NudgeStalled(proj, "nori", flowtest.Saying("idle"), StallDwell+time.Minute) {
		t.Error("an agent waiting on a verdict must not be nudged")
	}
	if len(deps.Injected) != 0 {
		t.Errorf("nothing should have been injected, got %v", deps.Injected)
	}
}

// TestNudgeStalledRechecksThePhase: the dwell is minutes old by definition, so the agent may have
// moved on while it elapsed. The state at nudge time is what decides.
func TestNudgeStalledRechecksThePhase(t *testing.T) {
	a, deps, ps := stallStore(t)
	flowtest.Place(t, ps, store.AgentState{Agent: "dvalin", Task: "td-d9a8c3", Phase: "submitted"})

	if a.NudgeStalled(proj, "dvalin", flowtest.Saying("idle"), StallDwell+time.Minute) {
		t.Error("an agent that moved on before the nudge landed must not be nudged")
	}
	if len(deps.Injected) != 0 {
		t.Errorf("nothing should have been injected, got %v", deps.Injected)
	}
}

// TestNudgeStalledLeavesAWorkerOnItsOwnQueuedRunAlone: a worker's `sindri lint`/`sindri run` returns
// at once and leaves it in phase "working" on purpose, so its pane goes idle for as long as the
// fleet's one queue slot takes to reach it — exactly what an unbounded pane read as a stall.
func TestNudgeStalledLeavesAWorkerOnItsOwnQueuedRunAlone(t *testing.T) {
	a, deps, ps := stallStore(t)
	if err := ps.PutRun(store.Run{ID: "run-1", Agent: "dvalin", Status: "queued"}); err != nil {
		t.Fatal(err)
	}
	if a.NudgeStalled(proj, "dvalin", flowtest.Saying("idle"), StallDwell+time.Minute) {
		t.Error("a worker waiting on its own queued run must not be nudged")
	}
	if len(deps.Injected) != 0 {
		t.Errorf("nothing should have been injected, got %v", deps.Injected)
	}
}

// TestNudgeStalledLeavesAReviewerOnAnAskedLintPRAlone: `sindri lint <pr>` returns a queued note and
// registers the reviewer as a waiter, exactly the case a reviewer raised as still unfixed here.
func TestNudgeStalledLeavesAReviewerOnAnAskedLintPRAlone(t *testing.T) {
	a, deps, ps := stallStore(t)
	reviewingAgent(t, ps, "ori", "pr-42")
	if err := ps.PutRun(store.Run{ID: "run-pr", Agent: "system", Status: "running", Kind: "lint-pr"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.AddRunWaiter("run-pr", "ori"); err != nil {
		t.Fatal(err)
	}
	if a.NudgeStalled(proj, "ori", flowtest.Saying("idle"), StallDwell+time.Minute) {
		t.Error("a reviewer waiting on a lint-pr run it asked for must not be nudged")
	}
	if len(deps.Injected) != 0 {
		t.Errorf("nothing should have been injected, got %v", deps.Injected)
	}
}

// TestNudgeStalledNeedsALiveAgent: a stopped pod has no session to type into.
func TestNudgeStalledNeedsALiveAgent(t *testing.T) {
	a, deps, _ := stallStore(t)
	deps.Down = true
	if a.NudgeStalled(proj, "dvalin", flowtest.SayingWhileDown("idle"), StallDwell+time.Minute) {
		t.Error("a down agent cannot be nudged")
	}
	if len(deps.Injected) != 0 {
		t.Errorf("nothing should have been injected, got %v", deps.Injected)
	}
}

// TestNudgeStalledIsLogged: the log is where a user reconstructs why an agent was prodded.
func TestNudgeStalledIsLogged(t *testing.T) {
	a, _, ps := stallStore(t)
	if !a.NudgeStalled(proj, "dvalin", flowtest.Saying("idle"), StallDwell+time.Minute) {
		t.Fatal("expected a nudge")
	}
	events, err := ps.Events("dvalin", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Type == "nudge" && strings.Contains(ev.Payload, "td-d9a8c3") {
			return
		}
	}
	t.Errorf("expected a logged nudge naming the task, got %+v", events)
}
