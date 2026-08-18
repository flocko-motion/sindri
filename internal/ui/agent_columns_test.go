package ui

import (
	"reflect"
	"testing"

	"github.com/flo-at/sindri/internal/ui/cli"
	"github.com/flo-at/sindri/internal/ui/tui"
)

// sharedCols keeps only the labels both front-ends' Agents tables carry, in the order given —
// the CLI's table also has "task" and "pr", the TUI's has "work", so a straight equality check
// would fail on those without saying anything about the columns this task cares about.
func sharedCols(labels []string) []string {
	want := map[string]bool{"repo": true, "agent": true, "role": true, "status": true}
	var out []string
	for _, l := range labels {
		if want[l] {
			out = append(out, l)
		}
	}
	return out
}

// TestAgentColumnOrderMatchesBothFrontEnds pins the Agents row's field order — repo, then agent,
// then role, then status — in both front-ends at once, reading each one's own table.Table (the
// same layout its rows render through), so a change to one side's column order without the other
// fails here instead of drifting unnoticed (sd-aadbb4 exists because it already had). Lives here,
// rather than inside tui or cli, because a test that imported both from either side would cycle
// back through cli's own import of tui.
func TestAgentColumnOrderMatchesBothFrontEnds(t *testing.T) {
	want := []string{"repo", "agent", "role", "status"}

	if got := sharedCols(tui.AgentColumnLabels()); !reflect.DeepEqual(got, want) {
		t.Errorf("TUI Agents columns: want %v, got %v", want, got)
	}
	if got := sharedCols(cli.AgentColumnLabels()); !reflect.DeepEqual(got, want) {
		t.Errorf("CLI Agents columns: want %v, got %v", want, got)
	}
}
