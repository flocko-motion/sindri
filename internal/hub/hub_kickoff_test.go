package hub

import (
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/container"
	hubagent "github.com/flo-at/sindri/internal/hub/harness"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/store"
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
	flowtest.Place(t, ps, store.AgentState{Agent: "dvalin", Phase: "idle"})
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
	if !strings.Contains(sent, prompts.DirPlanner) {
		t.Errorf("a launched planner should wake holding its own directive: %s", sent)
	}
	if strings.Contains(sent, "Run `sindri`") {
		t.Errorf("that fetch is the round trip this feature removes: %s", sent)
	}
}

// TestALaunchedWorkerIsGreetedWithItsDirective is the other half: the hub holds a worker's next job
// too, so the fetch asked it to ask for what the hub was holding as it spoke.
func TestALaunchedWorkerIsGreetedWithItsDirective(t *testing.T) {
	h, rt := greetable(t, "worker")
	h.greet(testProject, "dvalin")
	sent := rt.joined()
	if !strings.Contains(sent, prompts.DirNoTasks) {
		t.Errorf("a launched worker should wake knowing where it stands: %s", sent)
	}
	if strings.Contains(sent, "Run `sindri`") {
		t.Errorf("the round trip is gone for every role, not just the standing ones: %s", sent)
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
	if !strings.Contains(sent, prompts.DirPlanner) {
		t.Errorf("the kickoff behind the clear should carry the planner's directive: %s", sent)
	}
}

// TestAnArmedClearFiresAtTheBoundaryAndHandsThePlannerItsDirective is the third path and the one no
// human is present for: a clear armed while the agent held work fires later, from the planner's own
// clearing state, the moment it reaches a boundary.
func TestAnArmedClearFiresAtTheBoundaryAndHandsThePlannerItsDirective(t *testing.T) {
	h, rt := greetable(t, "planner")
	ps := h.store.For(testProject)
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	flowtest.Place(t, ps, store.AgentState{Agent: "dvalin", Task: "td-1", Phase: "planning"})
	if err := h.agents.SetClearArmed(t.Context(), testProject, "dvalin", true); err != nil {
		t.Fatalf("SetClearArmed: %v", err)
	}
	if sent := rt.joined(); strings.Contains(sent, "/clear") {
		t.Fatalf("precondition: an agent holding work must not be cleared yet: %s", sent)
	}
	flowtest.Place(t, ps, store.AgentState{Agent: "dvalin", Phase: "planning"})
	// A session with something in it: a clear typed into an empty one never lands, so the arming
	// waits for context to answer it with (-> cond.ClearArmed).
	h.watch.recordFill(store.Agent{Project: testProject, Name: "dvalin"}, fill{tokens: 40_000, window: 200_000})
	h.wf.Look(testProject, "dvalin")
	// Its own words on arrival: the conversation it was inside is what the clear discarded, so what
	// it is told is where it now stands rather than "carry on" with a session that is gone.
	if sent := rt.joined(); !strings.Contains(sent, prompts.DirPlanner) {
		t.Errorf("the clearing state's own kickoff should carry the planner's directive too: %s", sent)
	}
}

// TestTheDirectiveServesMailInlineAndMarksItRead: `sindri` is the one place every agent already
// looks, so the mail is served THERE, ahead of the ordinary directive that follows it in the same
// call — the ask itself is what marks it read, collapsing what used to be a "go read your mail,
// then ask again" detour into the one call an agent was already making.
func TestTheDirectiveServesMailInlineAndMarksItRead(t *testing.T) {
	h, ps := mailAgent(t)
	if _, err := ps.AddMail("dvalin", "hub", "[hub] td-1 was cancelled", false, 0); err != nil {
		t.Fatal(err)
	}
	dir, err := h.wf.AgentDirective(t.Context(), testProject, "dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, "td-1 was cancelled") {
		t.Errorf("the directive should carry the message itself: %q", dir)
	}
	if !strings.Contains(dir, "Work on task") {
		t.Errorf("and the ordinary directive should follow it in the same call: %q", dir)
	}
	// Read, not deleted — the record survives for a human.
	if n, _ := ps.UnreadMailCount("dvalin"); n != 0 {
		t.Errorf("asking should leave nothing unread, got %d", n)
	}
	if all, _ := h.store.AllMail(0); len(all) != 1 || !all[0].Read() {
		t.Errorf("the message must remain, marked read: %+v", all)
	}
}

// TestAUsersMailWaitsAndDoesNotPush is the whole of what makes it a second action rather than a mode
// of `tell`: choosing mail IS the choice not to interrupt, so it must not also be injected — and it
// must reach an agent that tell cannot, which is any agent with no live session.
func TestAUsersMailWaitsAndDoesNotPush(t *testing.T) {
	h, ps := mailAgent(t)
	if err := h.mail.MailAgent(testProject, "dvalin", "when you get to it, note that the base moved"); err != nil {
		t.Fatalf("MailAgent: %v", err)
	}
	unread, err := ps.UnreadMail("dvalin")
	if err != nil || len(unread) != 1 {
		t.Fatalf("the message must be waiting, got %+v (err %v)", unread, err)
	}
	if unread[0].Pushed {
		t.Error("a user's mail must not be pushed — not interrupting is the reason to pick it")
	}
	// Stamped as the user's, like `tell`, so the agent weights it as a human instruction and the Mail
	// view attributes it to a person rather than to the hub.
	if unread[0].Sender != "user" {
		t.Errorf("sender = %q, want user", unread[0].Sender)
	}
	if !strings.HasPrefix(unread[0].Body, "[user] ") {
		t.Errorf("the body should carry the provenance tag the agent reads: %q", unread[0].Body)
	}
	// And the agent is told at its next ask, served inline — which is what "it will be read" means
	// in practice, and the ask itself is what marks it read.
	dir, err := h.wf.AgentDirective(t.Context(), testProject, "dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, "note that the base moved") {
		t.Errorf("the directive should carry the message itself: %q", dir)
	}
	if again, err := ps.UnreadMail("dvalin"); err != nil || len(again) != 0 {
		t.Errorf("asking for the directive should have marked it read, got %+v (err %v)", again, err)
	}
}
