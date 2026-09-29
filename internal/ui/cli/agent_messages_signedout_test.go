package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestARefusalNamesWhatTheUserCanDo is the bug in CLI form: the message told the user that a
// restart makes the agent re-read its credentials, and then gave them no way to ask for one. Both
// answers are flags now, and the refusal names them.
func TestARefusalNamesWhatTheUserCanDo(t *testing.T) {
	a := &api.AgentView{Name: "eitri", Status: api.StatusSignedOut}
	_, err := tellSignedOut(a, false, false)
	if err == nil {
		t.Fatal("a signed-out agent must not be written to unasked")
	}
	for _, want := range []string{"--restart", "--anyway"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal must name %s, got %q", want, err)
		}
	}
}

// TestEitherAnswerIsCarriedOnTheMessage: the flags are the CLI's half of the TUI's modal, and each
// one travels to the hub as the answer it stands for.
func TestEitherAnswerIsCarriedOnTheMessage(t *testing.T) {
	a := &api.AgentView{Name: "eitri", Status: api.StatusSignedOut}
	if got, err := tellSignedOut(a, true, false); err != nil || got != api.SignedOutRestart {
		t.Errorf("--restart = (%q, %v), want the restart answer", got, err)
	}
	if got, err := tellSignedOut(a, false, true); err != nil || got != api.SignedOutSend {
		t.Errorf("--anyway = (%q, %v), want the send answer", got, err)
	}
	if _, err := tellSignedOut(a, true, true); err == nil {
		t.Error("--restart and --anyway ask for different things; asking for both must be refused")
	}
}

// TestAnAgentThatReadsFineIsToldWithoutCeremony: the question belongs to one state, and every
// other message carries no answer at all — so the hub's guard still stands behind it.
func TestAnAgentThatReadsFineIsToldWithoutCeremony(t *testing.T) {
	a := &api.AgentView{Name: "nori", Status: "working"}
	got, err := tellSignedOut(a, false, false)
	if err != nil || got != api.SignedOutRefuse {
		t.Errorf("telling a working agent = (%q, %v), want it sent with no answer attached", got, err)
	}
}
