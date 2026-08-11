package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestAgentPRMatchesThePRsTab: the PR the Agents tab shows against an author is one the PRs tab
// lists, so the two tabs never disagree about whether that agent has a PR at all. A scrapped one
// used to stick to its author on the Agents tab and be absent from the PRs tab.
func TestAgentPRMatchesThePRsTab(t *testing.T) {
	prs := []api.PR{
		{ID: "pr-1", Project: "p", Agent: "bombur", Status: "scrapped"},
		{ID: "pr-2", Project: "p", Agent: "dain", Status: "merged"},
		{ID: "pr-3", Project: "p", Agent: "jari", Status: "open"},
		{ID: "pr-4", Project: "p", Agent: "nori", Status: "rejected"},
	}
	for agent, want := range map[string]string{"bombur": "", "dain": "", "jari": "pr-3", "nori": "pr-4"} {
		if got := openPRFor(prs, "p", agent); got != want {
			t.Errorf("openPRFor(%s) = %q, want %q", agent, got, want)
		}
	}
}

// TestFullnessOnlyExplainsAnIdleAgent: "full" is a REASON an idle agent is passed over, not an
// activity. Applied unconditionally it overwrote the one fact the status column carries, so an
// agent mid-task read "full" — inviting the user to clear a context the hub refuses to clear at a
// leaf boundary, which is how they learned the board was lying.
func TestFullnessOnlyExplainsAnIdleAgent(t *testing.T) {
	for _, tc := range []struct {
		name              string
		status            string
		task, feature, pr string
		want              string
	}{
		{"idle and holding nothing is the one case", "idle", "", "", "", "full"},
		{"working keeps working", "working", "sd-1", "", "", "working"},
		{"blocked keeps blocked — it needs the user now", "blocked", "sd-1", "", "", "blocked"},
		{"submitted keeps submitted", "submitted", "sd-1", "", "pr-1", "submitted"},
		{"down keeps down", "down", "", "", "", "down"},
		// Stalled sits directly above fullness and has the same shape. A stalled agent HOLDS work,
		// and stalled is the more actionable word, so it must survive.
		{"stalled keeps stalled", "stalled", "sd-1", "", "", "stalled"},
		// The reason held work is checked directly: a quiet runtime probe reads a task-holder as
		// idle, and that agent is not idle in the sense fullness explains.
		{"idle but holding a task", "idle", "sd-1", "", "", "idle"},
		{"idle but holding a feature", "idle", "", "feat-1", "", "idle"},
		{"idle but carrying a PR", "idle", "", "", "pr-1", "idle"},
	} {
		if got := overlayFullness(tc.status, true, tc.task, tc.feature, tc.pr); got != tc.want {
			t.Errorf("%s: overlayFullness(%q) = %q, want %q", tc.name, tc.status, got, tc.want)
		}
	}
}

// TestNotFullChangesNothing: the overlay is only ever additive, so a fleet under the threshold
// reads exactly as it did before.
func TestNotFullChangesNothing(t *testing.T) {
	for _, status := range []string{"idle", "working", "blocked", "submitted", "stalled", "down"} {
		if got := overlayFullness(status, false, "", "", ""); got != status {
			t.Errorf("overlayFullness(%q, full=false) = %q, want it unchanged", status, got)
		}
	}
}
