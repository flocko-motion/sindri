package workflow

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestStalledOnlyCountsHeldWork is the whole rule. The phases that exist to wait must never read as
// stalled, or the nudge becomes noise on exactly the agents behaving correctly.
func TestStalledOnlyCountsHeldWork(t *testing.T) {
	past, under := StallDwell+time.Minute, StallDwell-time.Minute
	for _, c := range []struct {
		what           string
		phase, runtime string
		idleFor        time.Duration
		want           bool
	}{
		{"holding work and gone quiet", "working", "idle", past, true},
		{"quiet, but not for long enough", "working", "idle", under, false},
		{"thinking, not stalled", "working", "working", past, false},
		{"asking for input — that is 'blocked', already visible", "working", "blocked", past, false},
		{"waiting for a verdict on a submitted PR", "submitted", "idle", past, false},
		{"a container whose subtasks are all checkpointed", "", "idle", past, false},
		{"between assignments", "idle", "idle", past, false},
		{"probe told us nothing", "working", "", past, false},
	} {
		if got := Stalled(c.phase, c.runtime, c.idleFor); got != c.want {
			t.Errorf("%s: Stalled(%q, %q, %v) = %v, want %v", c.what, c.phase, c.runtime, c.idleFor, got, c.want)
		}
	}
}

// stallStore is one worker holding a task, and one waiting on a verdict.
func stallStore(t *testing.T) (*Engine, *stubDeps, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("proj")
	for _, a := range []store.Agent{
		{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"},
		{Name: "nori", Role: "worker", Workspace: ".worktrees/nori"},
	} {
		if err := ps.PutAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	_ = ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-d9a8c3", Branch: "td-d9a8c3", Phase: "working"})
	_ = ps.SetState(store.AgentState{Agent: "nori", Task: "td-other", Branch: "td-other", Phase: "submitted"})
	deps := &stubDeps{root: t.TempDir(), alive: true}
	return New(st, deps), deps, ps
}

// TestNudgeStalledNamesTheTask: a stalled agent has lost the thread, so the prod has to say which
// task it still holds and offer the other honest answer — naming a blocker instead of sitting on it.
func TestNudgeStalledNamesTheTask(t *testing.T) {
	e, deps, _ := stallStore(t)

	if !e.NudgeStalled("proj", "dvalin", "idle", StallDwell+time.Minute) {
		t.Fatal("a worker holding work and gone quiet should be nudged")
	}
	if len(deps.injected) != 1 || deps.injected[0] != "dvalin" {
		t.Fatalf("expected dvalin to be nudged, got %v", deps.injected)
	}
	got := deps.injectedText[0]
	for _, want := range []string{"td-d9a8c3", "blocks you"} {
		if !strings.Contains(got, want) {
			t.Errorf("the nudge should mention %q, got:\n%s", want, got)
		}
	}
}

// TestNudgeStalledLeavesWaitingAgentsAlone: "submitted" is waiting for a verdict the agent cannot
// hurry, so prodding it would be telling it off for doing the right thing.
func TestNudgeStalledLeavesWaitingAgentsAlone(t *testing.T) {
	e, deps, _ := stallStore(t)
	if e.NudgeStalled("proj", "nori", "idle", StallDwell+time.Minute) {
		t.Error("an agent waiting on a verdict must not be nudged")
	}
	if len(deps.injected) != 0 {
		t.Errorf("nothing should have been injected, got %v", deps.injected)
	}
}

// TestNudgeStalledRechecksThePhase: the dwell is minutes old by definition, so the agent may have
// moved on while it elapsed. The state at nudge time is what decides.
func TestNudgeStalledRechecksThePhase(t *testing.T) {
	e, deps, ps := stallStore(t)
	_ = ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-d9a8c3", Phase: "submitted"})

	if e.NudgeStalled("proj", "dvalin", "idle", StallDwell+time.Minute) {
		t.Error("an agent that moved on before the nudge landed must not be nudged")
	}
	if len(deps.injected) != 0 {
		t.Errorf("nothing should have been injected, got %v", deps.injected)
	}
}

// TestNudgeStalledNeedsALiveAgent: a stopped pod has no session to type into.
func TestNudgeStalledNeedsALiveAgent(t *testing.T) {
	e, deps, _ := stallStore(t)
	deps.alive = false
	if e.NudgeStalled("proj", "dvalin", "idle", StallDwell+time.Minute) {
		t.Error("a down agent cannot be nudged")
	}
	if len(deps.injected) != 0 {
		t.Errorf("nothing should have been injected, got %v", deps.injected)
	}
}

// TestNudgeStalledIsLogged: the log is where a user reconstructs why an agent was prodded.
func TestNudgeStalledIsLogged(t *testing.T) {
	e, _, ps := stallStore(t)
	if !e.NudgeStalled("proj", "dvalin", "idle", StallDwell+time.Minute) {
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
