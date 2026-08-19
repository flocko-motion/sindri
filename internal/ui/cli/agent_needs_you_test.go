package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestNeedsYouSummaryNamesEveryStuckAgent is the CLI's half of the Agents marker. A row says
// "blocked" or "stalled" in a status column a reader skims; the summary says whose move it is, so a
// user who never opens the TUI still learns the fleet has stopped on them.
func TestNeedsYouSummaryNamesEveryStuckAgent(t *testing.T) {
	got := needsYouSummary([]api.AgentView{
		{Name: "dvalin", Status: api.StatusBlocked},
		{Name: "gloin", Status: api.StatusSignedOut},
		{Name: "nori", Status: api.StatusStalled},
		{Name: "fili", Status: "idle"},
		{Name: "kili", Status: "working"},
	})
	if !strings.HasPrefix(got, "3 agent(s) need you") {
		t.Errorf("summary should count the three stuck agents, got %q", got)
	}
	for _, name := range []string{"dvalin", "gloin", "nori"} {
		if !strings.Contains(got, name) {
			t.Errorf("summary should name %s — a count alone leaves the user opening panes to find it: %q", name, got)
		}
	}
	for _, name := range []string{"fili", "kili"} {
		if strings.Contains(got, name) {
			t.Errorf("summary should leave %s alone: %q", name, got)
		}
	}
}

// TestAnEscalatedAgentIsQuotedNotJustNamed: for the other states the remedy is in the status word,
// so naming the agent is enough. An escalation carries a question, and the question IS what the user
// has to act on — having to attach to each pane to read it is what makes triaging several expensive.
func TestAnEscalatedAgentIsQuotedNotJustNamed(t *testing.T) {
	const q = "drop the two callers or keep both?"
	got := needsYouSummary([]api.AgentView{
		{Name: "dvalin", Status: api.StatusEscalated, Escalation: q},
		{Name: "kili", Status: "working"},
	})
	if !strings.Contains(got, q) {
		t.Errorf("the summary should carry the question itself: %q", got)
	}
	// And it says how to answer it, since the remedies for the other states do not apply here.
	if !strings.Contains(got, "sindri agent resume") {
		t.Errorf("the summary should name the release for one that cannot resume itself: %q", got)
	}
}

// TestNeedsYouSummarySilentWhenNothingIsStuck: the line is a call to act, so a healthy fleet
// prints nothing rather than a reassurance the user has to read past every time.
func TestNeedsYouSummarySilentWhenNothingIsStuck(t *testing.T) {
	if got := needsYouSummary([]api.AgentView{
		{Name: "fili", Status: "idle"},
		{Name: "kili", Status: "working", Task: "sd-1"},
	}); got != "" {
		t.Errorf("nothing waiting on the user should print nothing, got %q", got)
	}
}
