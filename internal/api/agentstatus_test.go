package api

import "testing"

// TestUnknownIsNeverMistakenForRunning is the regression this pair exists to prevent. "unknown"
// means no evidence, but every consumer decided with a switch that ended in a default meaning
// "running" — so a word minted to say "we have not looked" was read as "it is up" by four call
// sites at once, and `sindri coauthor` attached to a pod it had skipped launching.
func TestUnknownIsNeverMistakenForRunning(t *testing.T) {
	if !AgentNotUp(StatusUnknown) {
		t.Error("an unobserved agent must never be treated as one to act on")
	}
	if !AgentNeedsLaunch(StatusUnknown) {
		t.Error("an unobserved agent must be launchable — this is where 'down' used to send it")
	}
}

// TestAgentNotUpCoversEveryNonRunningWord: the point of the helper is that the list lives once.
func TestAgentNotUpCoversEveryNonRunningWord(t *testing.T) {
	for _, s := range []string{"", "down", StatusUnknown, "launching", "stopping"} {
		if !AgentNotUp(s) {
			t.Errorf("AgentNotUp(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"idle", "working", "blocked", "submitted", "collab", "stalled", "full"} {
		if AgentNotUp(s) {
			t.Errorf("AgentNotUp(%q) = true — a running agent must stay actionable", s)
		}
	}
}

// TestNeedsLaunchExcludesWhatIsAlreadyMoving: relaunching an agent whose launch is in flight would
// double up, which is why this is narrower than AgentNotUp rather than the same predicate.
func TestNeedsLaunchExcludesWhatIsAlreadyMoving(t *testing.T) {
	for _, s := range []string{"launching", "stopping", "idle", "working", ""} {
		if AgentNeedsLaunch(s) {
			t.Errorf("AgentNeedsLaunch(%q) = true, want false", s)
		}
	}
	if !AgentNeedsLaunch("down") {
		t.Error("a down agent still needs launching")
	}
}
