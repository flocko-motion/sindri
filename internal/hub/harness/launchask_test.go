package harness

import (
	"strings"
	"testing"
)

// TestTheMachinesLaunchCarriesOutTheHeldRequest: the /launch route only asks, so whether the launch
// worked has to come back to it from the machine's Start. The fake preflight's failure is the result
// seen landing.
func TestTheMachinesLaunchCarriesOutTheHeldRequest(t *testing.T) {
	s, _ := tellFixture(t, "eitri", idlePane) // fakeRuntime.Check always fails
	ask := s.AskLaunch("proj", "eitri", LaunchOpts{}, nil)
	err := s.Start(t.Context(), "proj", "eitri")
	if err == nil || !strings.Contains(err.Error(), "nothing to launch into") {
		t.Fatalf("Start = %v, want it to fail at the fake preflight", err)
	}
	ran, got := ask.Result()
	if !ran || got == nil || got.Error() != err.Error() {
		t.Errorf("the asker read ran=%v err=%v, want the launch's own %v", ran, got, err)
	}
}

// TestAWithdrawnRequestIsNotCarriedOut: a caller that stopped waiting leaves nothing for a later
// launch to take — the machine starting the agent on its own must not run a shell someone once asked for.
func TestAWithdrawnRequestIsNotCarriedOut(t *testing.T) {
	s, _ := tellFixture(t, "eitri", idlePane)
	ask := s.AskLaunch("proj", "eitri", LaunchOpts{Shell: true}, nil)
	s.Withdraw("proj", "eitri", ask)
	_ = s.Start(t.Context(), "proj", "eitri")
	if ran, _ := ask.Result(); ran {
		t.Error("a withdrawn request was carried out")
	}
}
