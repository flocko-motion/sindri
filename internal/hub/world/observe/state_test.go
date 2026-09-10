package observe

import "testing"

// TestEveryStateRoundTripsThroughItsWord: the words are the coding tool's, and the board renders
// them back, so a state that could not survive the trip would show as blank on somebody's screen.
func TestEveryStateRoundTripsThroughItsWord(t *testing.T) {
	for _, s := range []State{Working, AtPrompt, AwaitingHuman, SignedOut, TurnCutOff} {
		word := s.String()
		if word == "" {
			t.Errorf("state %d has no word, so the board would render it blank", s)
			continue
		}
		if got := ParseState(word); got != s {
			t.Errorf("ParseState(%q) = %v, want %v — the two directions disagree", word, got, s)
		}
	}
}

// TestAnUnreadWordIsUnknownNotIdle is the trap the type exists for. A capture that failed, or a word
// from a newer tool, is NO EVIDENCE — and reading it as "at a prompt" is the same error as reading an
// unobserved agent as down: a claim nothing supports.
func TestAnUnreadWordIsUnknownNotIdle(t *testing.T) {
	for _, word := range []string{"", "thinking", "IDLE", "signed out"} {
		if got := ParseState(word); got != Unknown {
			t.Errorf("ParseState(%q) = %v, want Unknown", word, got)
		}
	}
	if (Observation{}).AtPrompt() {
		t.Error("the zero observation must not read as sitting at a prompt")
	}
	if Unknown.String() != "" {
		t.Errorf("Unknown renders as %q, want the same empty a failed capture reports", Unknown.String())
	}
}

// TestTheStateIsNotAString is what makes the seam hold by construction rather than by review: a
// caller cannot compare a state to a word it invented, because the type will not allow it. This
// test states the property; the compiler enforces it, and `State("idle")` does not build.
func TestTheStateIsNotAString(t *testing.T) {
	var s State = AtPrompt
	if _, isString := any(s).(string); isString {
		t.Fatal("State is a string type — a bare literal would compare equal to it, which is the hole " +
			"this type closes (-> the words leaked into stall.go, stallwatch.go and credwatch.go)")
	}
}
