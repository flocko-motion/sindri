package harness

import "testing"

// TestTheModelSwitchDoesNotClearASessionJustCleared is the sequence preparation actually runs: the
// session is cleared, and THEN put on its work's model. By the time the switch runs there is nothing
// left to drop — but a fresh session reports a few tokens rather than none (-> awaitCleared), so a
// switch that reads "there is context here" clears a second time and then waits out a fall that
// cannot come. That wait ends in a timeout, and a timed-out switch stops the agent for a human.
func TestTheModelSwitchDoesNotClearASessionJustCleared(t *testing.T) {
	s, f := modelFixture(t)
	writeUsageOn(t, "proj", "durin", 80_000, "claude-sonnet-5")
	s.ForgetContext("proj", "durin")
	// The clear landing the way a real one does: a NEW session, carrying the few tokens every fresh
	// one carries rather than nothing at all.
	f.afterSubmit = func() {
		if len(f.sent) == 1 && f.sent[0] == "/clear" {
			writeUsageOn(t, "proj", "durin", 1_200, "claude-sonnet-5")
		}
	}
	if err := s.Clear(t.Context(), "proj", "durin"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	f.afterSubmit = nil

	if err := s.SetTier(t.Context(), "proj", "durin", "senior"); err != nil {
		t.Fatalf("SetTier onto a session just cleared: %v", err)
	}

	want := []string{"/clear", "/model claude-opus-5"}
	if len(f.sent) != len(want) {
		t.Fatalf("sent = %v, want %v — the switch cleared a session the step before it had emptied", f.sent, want)
	}
	for i, w := range want {
		if f.sent[i] != w {
			t.Errorf("sent[%d] = %q, want %q", i, f.sent[i], w)
		}
	}
}
