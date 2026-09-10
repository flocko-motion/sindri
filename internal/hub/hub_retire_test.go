// package: hub / retire_test
// type:    logic (what winding an agent down says to it, and what it stops saying)
// job:     pin both directions of the retire verb — retiring is silent, because an agent told to
// stop needs no announcement, while bringing one back must reach it wherever it was parked.
// limits:  the messaging around the verb. WHICH automatic behaviours retirement suspends is the
// surface's rule (-> situation.Situation.ParkedByTheHub).
package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/observe"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestUnretiringTellsTheAgent is sd-9d36d2: DirRetired sends an agent away from ever asking again,
// so the push on the false transition of SetRetired is the only channel that would reach it.
func TestUnretiringTellsTheAgent(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Retired: true}); err != nil {
		t.Fatal(err)
	}
	if err := h.SetRetired(testProject, "dvalin", false); err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	unread, err := ps.UnreadMail("dvalin")
	if err != nil || len(unread) != 1 {
		t.Fatalf("un-retiring should have mailed the agent, got %+v (err %v)", unread, err)
	}
	if !strings.Contains(unread[0].Body, "back in service") {
		t.Errorf("the message should say it is back in service: %q", unread[0].Body)
	}
}

// TestRetiringSaysNothingToTheAgent: winding an agent down is not itself news to push — DirRetired
// is served on its next ordinary ask, same as any other resting directive.
func TestRetiringSaysNothingToTheAgent(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := h.SetRetired(testProject, "dvalin", true); err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	if all, _ := h.store.AllMail(0); len(all) != 0 {
		t.Errorf("retiring should not mail the agent, got %+v", all)
	}
}

// TestUnretiringAnAlreadyActiveAgentIsANoOp: `--back` on an agent that was never retired is not a
// real transition, so it must not manufacture a wake nobody needs.
func TestUnretiringAnAlreadyActiveAgentIsANoOp(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := h.SetRetired(testProject, "dvalin", false); err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	if all, _ := h.store.AllMail(0); len(all) != 0 {
		t.Errorf("un-retiring one that was never retired should be quiet, got %+v", all)
	}
}

// loggedPushSuppressed reports whether a suppressed-push event was recorded for the agent.
func loggedPushSuppressed(t *testing.T, h *Hub, agent string) bool {
	t.Helper()
	evs, err := h.store.For(testProject).Events(agent, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Type == "push-suppressed" {
			return true
		}
	}
	return false
}

// TestUnretireStillReachesTheAgent: SetRetired flips the flag before delivering MsgUnretired, so by
// push time WakeRefusal's own retired-check already reads false — no exemption needed for this case.
func TestUnretireStillReachesTheAgent(t *testing.T) {
	h, ps := mailAgent(t)
	a, _, err := ps.GetAgent("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	if err := h.SetRetired(testProject, "dvalin", false); err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	unread, err := ps.UnreadMail("dvalin")
	if err != nil || len(unread) != 1 {
		t.Fatalf("the un-retire notice must land, got %+v (err %v)", unread, err)
	}
	if loggedPushSuppressed(t, h, "dvalin") {
		t.Error("un-retiring must not suppress its own notification")
	}
}

// TestUnretiringAnEscalatedAgentStillWaitsOnTheEscalation is TestUnretireStillReachesTheAgent's
// converse: retirement and escalation are independent, so un-retiring must not itself wake an agent
// still stuck on a decision — that decision, not the retirement, is what it is actually waiting on.
func TestUnretiringAnEscalatedAgentStillWaitsOnTheEscalation(t *testing.T) {
	h, ps := mailAgent(t)
	a, _, err := ps.GetAgent("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if _, err := h.AgentFlow().Escalate(testProject, "dvalin", "one column or two?"); err != nil {
		t.Fatal(err)
	}

	if err := h.SetRetired(testProject, "dvalin", false); err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	unread, err := ps.UnreadMail("dvalin")
	if err != nil || len(unread) != 1 {
		t.Fatalf("the un-retire notice must still be recorded as mail, got %+v (err %v)", unread, err)
	}
	if unread[0].Pushed {
		t.Error("an escalated agent must not be woken just to hear it is un-retired")
	}
	if !loggedPushSuppressed(t, h, "dvalin") {
		t.Error("the suppression must be logged, same as any other gated push")
	}
}

// TestKickoffReachesARetiredAgent pins the review-round finding: rehydrate's kickoff is the ONLY way
// a fresh session ever learns to call sindri at all — gated and mail-less, a retired agent's pod would
// sit silent forever instead of seeing DirRetired even once.
func TestKickoffReachesARetiredAgent(t *testing.T) {
	h, ps := mailAgent(t)
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	a, _, err := ps.GetAgent("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	// No real pod is running here, so the push still fails to land — the log is what proves the gate
	// itself let it through rather than refusing on sight.
	_ = h.mail.Deliver(testProject, "dvalin", prompts.MsgKickoff, mail.PushOnly)
	if loggedPushSuppressed(t, h, "dvalin") {
		t.Error("a retired agent's kickoff must not be gated — it is the only way it learns that")
	}
}

// TestEscalatedAgentIsStillNudgedAboutWaitingMail is the actual incident: NudgeMailWaiting's push is
// the only thing that wakes an idle, escalated agent to read a user's answer — through the real Deliver.
func TestEscalatedAgentIsStillNudgedAboutWaitingMail(t *testing.T) {
	h, ps := mailAgent(t)
	if _, err := ps.AddMail("dvalin", "user", "one column, ship it", false, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := h.AgentFlow().Escalate(testProject, "dvalin", "one column or two?"); err != nil {
		t.Fatal(err)
	}
	// Fake the pane as an idle, running session — what a real watchdog sweep would have recorded.
	h.watch.mu.Lock()
	h.watch.obs[agentKey{testProject, "dvalin"}] = liveness{up: true, state: observe.AtPrompt}
	h.watch.mu.Unlock()

	// No real pod is running here, so the push itself cannot land — what this proves is narrower but
	// exactly the bug: it must fail for lack of a pane, never because WakeRefusal gated it.
	h.mail.NudgeMailWaiting(testProject, "dvalin")
	if loggedPushSuppressed(t, h, "dvalin") {
		t.Error("the mail-waiting push must not be gated on the very state it exists to end")
	}
}
