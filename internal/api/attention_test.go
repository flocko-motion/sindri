package api

import "testing"

// TestNeedsUserIsEveryStateOnlyAHumanClears walks the states an agent can wear. The four that count
// share one property and not a family resemblance: nothing the agent or the hub does resolves them.
func TestNeedsUserIsEveryStateOnlyAHumanClears(t *testing.T) {
	for _, s := range []string{StatusBlocked, StatusSignedOut, StatusFull, StatusStalled} {
		if !AgentNeedsUser(AgentView{Status: s}) {
			t.Errorf("AgentNeedsUser(%q) = false — that agent holds its work and nobody but a human can move it", s)
		}
	}
	for _, s := range []string{"idle", "working", "submitted", "planning", "collab", "reviewing", "down", StatusUnknown, ""} {
		if AgentNeedsUser(AgentView{Status: s}) {
			t.Errorf("AgentNeedsUser(%q) = true — marking this makes the glyph mean 'an agent exists'", s)
		}
	}
}

// TestIdleIsNeverTheUsersProblem is the line the marker and the idle observer share. An agent
// waiting for work is functioning normally; one idle beside work it could claim is a dispatch fault
// the hub nudges. Neither asks anything of the user, so neither may be counted.
func TestIdleIsNeverTheUsersProblem(t *testing.T) {
	if AgentNeedsUser(AgentView{Status: "idle"}) {
		t.Error("an idle agent is waiting for work, not for the user")
	}
}

// TestRetiredNeverCounts pins the states retirement actually reaches. A retired agent keeps
// running: it fills its context and reads "full", or holds work, stands still and reads "stalled".
// Both pass the switch, so retirement has to be tested against THOSE — against "idle" the case
// passes whatever Retired says, and proves nothing. Retiring a full worker is the ordinary way to
// wind one down, and a marker that stuck to it would sit on the handle until it was deleted.
func TestRetiredNeverCounts(t *testing.T) {
	for _, s := range []string{StatusFull, StatusStalled, StatusBlocked, StatusSignedOut} {
		if !AgentNeedsUser(AgentView{Status: s}) {
			t.Fatalf("%q must count while running, or this test proves nothing about retirement", s)
		}
		if AgentNeedsUser(AgentView{Status: s, Retired: true}) {
			t.Errorf("a retired agent reading %q was wound down deliberately; the decision is made", s)
		}
	}
}

// TestCountAgentsNeedingUserCountsAgentsNotReasons: an agent both full and stalled wears one word,
// and either way it is one thing to attend to.
func TestCountAgentsNeedingUserCountsAgentsNotReasons(t *testing.T) {
	got := CountAgentsNeedingUser([]AgentView{
		{Name: "a", Status: StatusStalled, ContextTokens: 190_000, ContextWindow: 200_000},
		{Name: "b", Status: "working"},
		{Name: "c", Status: StatusBlocked},
	})
	if got != 2 {
		t.Errorf("CountAgentsNeedingUser = %d, want 2", got)
	}
}
