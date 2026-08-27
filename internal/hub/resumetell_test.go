package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestResumingAnAgentTellsIt is the gap: clearing an escalation from the host moved the board and
// nothing else. The agent stopped because it was told to, so it sat there — sudri did, for hours,
// after the fault it escalated on had already been fixed.
func TestResumingAnAgentTellsIt(t *testing.T) {
	h, ps := mailAgent(t)
	if _, err := h.Escalate(testProject, "dvalin", "one column or two?"); err != nil {
		t.Fatal(err)
	}
	if err := h.ResumeByUser(testProject, "dvalin", "one column, ship it"); err != nil {
		t.Fatalf("ResumeByUser: %v", err)
	}

	if st, _ := ps.GetState("dvalin"); st.Escalation != "" {
		t.Errorf("the escalation should be cleared, got %q", st.Escalation)
	}
	unread, err := ps.UnreadMail("dvalin")
	if err != nil || len(unread) != 1 {
		t.Fatalf("the release must reach the agent, got %+v (err %v)", unread, err)
	}
	got := unread[0].Body
	// The question comes back with it: the agent may have been cleared or restarted since raising it,
	// and "you may carry on" alone leaves it guessing what was settled.
	for _, want := range []string{"one column or two?", "one column, ship it", "sindri"} {
		if !strings.Contains(got, want) {
			t.Errorf("the notice is missing %q:\n%s", want, got)
		}
	}
	if unread[0].Sender != api.SenderUser {
		t.Errorf("sender = %q; the user cleared it, and provenance is stated", unread[0].Sender)
	}
}

// TestResumingWithoutAnAnswerStillTells: a fault the user simply went and fixed leaves nothing to
// answer, and the agent still has to hear that it may move.
func TestResumingWithoutAnAnswerStillTells(t *testing.T) {
	h, ps := mailAgent(t)
	if _, err := h.Escalate(testProject, "dvalin", "`sindri git` failed inside the hub"); err != nil {
		t.Fatal(err)
	}
	if err := h.ResumeByUser(testProject, "dvalin", ""); err != nil {
		t.Fatalf("ResumeByUser: %v", err)
	}
	unread, _ := ps.UnreadMail("dvalin")
	if len(unread) != 1 {
		t.Fatalf("want one notice, got %d", len(unread))
	}
	if strings.Contains(unread[0].Body, "Their answer:") {
		t.Errorf("an empty answer must not render an empty section:\n%s", unread[0].Body)
	}
}

// TestResumingAnUnescalatedAgentSaysNothing: the clear is a no-op there, and mail is never deleted —
// a notice about an escalation that never happened would be permanent noise.
func TestResumingAnUnescalatedAgentSaysNothing(t *testing.T) {
	h, ps := mailAgent(t)
	if err := h.ResumeByUser(testProject, "dvalin", "carry on"); err != nil {
		t.Fatalf("ResumeByUser: %v", err)
	}
	if unread, _ := ps.UnreadMail("dvalin"); len(unread) != 0 {
		t.Errorf("nothing was cleared, so nothing should be announced, got %+v", unread)
	}
}

// TestTheResumeNoticeIsNotEatenByTheWakeGate: a retired agent is exactly one the gate refuses to
// wake, and the release from a stop is the one push that has to reach it anyway.
func TestTheResumeNoticeIsNotEatenByTheWakeGate(t *testing.T) {
	h, ps := mailAgent(t)
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Phase: "idle"}, store.ReasonFreed, "test setup"); err != nil {
		t.Fatal(err)
	}
	a, _, _ := ps.GetAgent("dvalin")
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Escalate(testProject, "dvalin", "one column or two?"); err != nil {
		t.Fatal(err)
	}
	if err := h.ResumeByUser(testProject, "dvalin", "one column"); err != nil {
		t.Fatalf("ResumeByUser: %v", err)
	}
	// No real pod is running here, so the push fails to land regardless; the log is what proves the
	// gate let it through rather than refusing on sight.
	if loggedPushSuppressed(t, h, "dvalin") {
		t.Error("the notice that ends a stop must not be gated by the state it ends")
	}
}
