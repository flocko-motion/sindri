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
	for _, s := range []string{"", "down", "stopped", StatusUnknown, "launching", "stopping", StatusLaunchFailed} {
		if !AgentNotUp(s) {
			t.Errorf("AgentNotUp(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"idle", "working", "blocked", "submitted", "collab", "stalled", StatusEscalated} {
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
	if !AgentNeedsLaunch("stopped") {
		t.Error("a deliberately stopped agent needs launching too — the same verb resumes it")
	}
	if !AgentNeedsLaunch(StatusLaunchFailed) {
		t.Error("a failed launch needs launching too — the remedy is the same keystroke")
	}
}

// TestClearWaitsForNamesTheWorkInHand is the "when" both front-ends state on the same board: a leaf
// task defers the clear, a held feature does not (it fires between subtasks), and a reviewer's open
// review does. The rule lives here so the confirm and the CLI cannot describe the same agent
// differently.
func TestClearWaitsForNamesTheWorkInHand(t *testing.T) {
	cases := []struct {
		name  string
		agent AgentView
		want  string
	}{
		{"idle", AgentView{Role: "worker", Status: "idle"}, ""},
		{"working", AgentView{Role: "worker", Task: "sd-1"}, "sd-1"},
		{"between subtasks", AgentView{Role: "worker", Feature: "sd-epic"}, ""},
		{"mid-subtask", AgentView{Role: "worker", Feature: "sd-epic", Task: "sd-2"}, "sd-2"},
		{"reviewing", AgentView{Role: "reviewer", PR: "pr-1"}, "pr-1"},
		{"reviewer at rest", AgentView{Role: "reviewer"}, ""},
		{"planner", AgentView{Role: "planner"}, ""},
		// A worker's PR is its own submitted work, and it still holds the task — which is what
		// defers the clear. The PR must not be read as a review it owes.
		{"worker awaiting a verdict", AgentView{Role: "worker", Task: "sd-3", PR: "pr-2"}, "sd-3"},
	}
	for _, c := range cases {
		if got := ClearWaitsFor(c.agent); got != c.want {
			t.Errorf("%s: ClearWaitsFor = %q, want %q", c.name, got, c.want)
		}
	}
}
