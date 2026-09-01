package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/container"
	hubagent "github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// greetable seeds one idle agent in role, observed as up, with a fake runtime recording everything
// typed into its session — so what the hub SENDS a fresh session is readable off rt.
func greetable(t *testing.T, role string) (*Hub, *clearableRuntime) {
	t.Helper()
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: role}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	w := stillWatchdog(t, h)
	w.record(store.Agent{Project: testProject, Name: "dvalin"}, true, 0, hubagent.Observation{Runtime: "idle", Digest: "d1"})
	rt := &clearableRuntime{}
	container.Use(rt)
	t.Cleanup(container.UseDefault)
	return h, rt
}

// TestALaunchedPlannerIsGreetedWithItsDirective pins the wiring, not the choice: Engine.Kickoff can be
// right in every branch and reach nothing, which is what the launch path did before — a planner's pod
// came up on MsgKickoff and spent a call and a turn to hear "carry on with the user", eight times in
// one session. greet rather than rehydrate, to skip its 8-second boot wait.
func TestALaunchedPlannerIsGreetedWithItsDirective(t *testing.T) {
	h, rt := greetable(t, "planner")
	h.greet(testProject, "dvalin")
	sent := rt.joined()
	if !strings.Contains(sent, workflow.DirPlanner) {
		t.Errorf("a launched planner should wake holding its own directive: %s", sent)
	}
	if strings.Contains(sent, workflow.MsgKickoff) {
		t.Errorf("that fetch is the round trip this feature removes: %s", sent)
	}
}

// TestALaunchedWorkerIsStillSentToFetch is the other half: the hub holds a worker's next job, so its
// answer varies and the call buys the current one.
func TestALaunchedWorkerIsStillSentToFetch(t *testing.T) {
	h, rt := greetable(t, "worker")
	h.greet(testProject, "dvalin")
	if sent := rt.joined(); !strings.Contains(sent, workflow.MsgKickoff) {
		t.Errorf("a launched worker should still be told to run `sindri`: %s", sent)
	}
}

// TestAClearedPlannerIsHandedItsDirective covers the second delivery path: a context clear re-serves
// the kickoff, and a planner sent to fetch there pays the same round trip on an empty context.
func TestAClearedPlannerIsHandedItsDirective(t *testing.T) {
	h, rt := greetable(t, "planner")
	if err := h.agents.SetClearArmed(t.Context(), testProject, "dvalin", true); err != nil {
		t.Fatalf("SetClearArmed: %v", err)
	}
	sent := rt.joined()
	if !strings.Contains(sent, "/clear") {
		t.Fatalf("precondition: the session was never cleared: %s", sent)
	}
	if !strings.Contains(sent, workflow.DirPlanner) {
		t.Errorf("the kickoff behind the clear should carry the planner's directive: %s", sent)
	}
}

// TestTheArmedClearSweepHandsThePlannerItsDirective is the third path and the one no human is present
// for: a clear armed while the agent held work fires later off the hub's own tick.
func TestTheArmedClearSweepHandsThePlannerItsDirective(t *testing.T) {
	h, rt := greetable(t, "planner")
	ps := h.store.For(testProject)
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-1", Phase: "planning"}, store.ReasonClaimed, "holds work"); err != nil {
		t.Fatal(err)
	}
	if err := h.agents.SetClearArmed(t.Context(), testProject, "dvalin", true); err != nil {
		t.Fatalf("SetClearArmed: %v", err)
	}
	if sent := rt.joined(); strings.Contains(sent, "/clear") {
		t.Fatalf("precondition: an agent holding work must not be cleared yet: %s", sent)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Phase: "planning"}, store.ReasonFreed, "reached a boundary"); err != nil {
		t.Fatal(err)
	}
	h.agents.FireArmedClears(t.Context(), testProject)
	if sent := rt.joined(); !strings.Contains(sent, workflow.DirPlanning) {
		t.Errorf("the sweep's kickoff should carry the planner's directive too: %s", sent)
	}
}
