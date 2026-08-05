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
