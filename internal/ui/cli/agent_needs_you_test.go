package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestNeedsYouSummaryNamesEveryStuckAgent is the CLI's half of the Agents marker. A row says
// "blocked" or "full" in a status column a reader skims; the summary says whose move it is, so a
// user who never opens the TUI still learns the fleet has stopped on them.
func TestNeedsYouSummaryNamesEveryStuckAgent(t *testing.T) {
	got := needsYouSummary([]api.AgentView{
		{Name: "dvalin", Status: api.StatusBlocked},
		{Name: "gloin", Status: api.StatusSignedOut},
		{Name: "bombur", Status: api.StatusFull},
		{Name: "nori", Status: api.StatusStalled},
		{Name: "fili", Status: "idle"},
		{Name: "kili", Status: "working"},
	})
	if !strings.HasPrefix(got, "4 agent(s) need you") {
		t.Errorf("summary should count the four stuck agents, got %q", got)
	}
	for _, name := range []string{"dvalin", "gloin", "bombur", "nori"} {
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
