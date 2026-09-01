package hub

import (
	"github.com/flo-at/sindri/internal/hub/observe"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

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
	if _, err := h.Escalate(testProject, "dvalin", "one column or two?"); err != nil {
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
	_ = h.Deliver(testProject, "dvalin", workflow.MsgKickoff, workflow.PushOnly)
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
	if _, err := h.Escalate(testProject, "dvalin", "one column or two?"); err != nil {
		t.Fatal(err)
	}
	// Fake the pane as an idle, running session — what a real watchdog sweep would have recorded.
	h.watch.mu.Lock()
	h.watch.obs[agentKey{testProject, "dvalin"}] = liveness{up: true, state: observe.AtPrompt}
	h.watch.mu.Unlock()

	// No real pod is running here, so the push itself cannot land — what this proves is narrower but
	// exactly the bug: it must fail for lack of a pane, never because WakeRefusal gated it.
	h.wf.NudgeMailWaiting(testProject, "dvalin")
	if loggedPushSuppressed(t, h, "dvalin") {
		t.Error("the mail-waiting push must not be gated on the very state it exists to end")
	}
}
