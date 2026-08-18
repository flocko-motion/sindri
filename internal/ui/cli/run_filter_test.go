package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestRunListFilterFlagDefaultsToTodaysBehaviour mirrors TestPRListFilterFlagDefaultsToTodaysBehaviour.
func TestRunListFilterFlagDefaultsToTodaysBehaviour(t *testing.T) {
	f := runListCmd().Flags().Lookup("filter")
	if f == nil {
		t.Fatal("`run list` must offer --filter; the TUI cycles the same set with `f`")
	}
	if f.DefValue != string(api.RunFilterAll) {
		t.Errorf("--filter defaults to %q, want %q — the bare command must keep printing every run",
			f.DefValue, api.RunFilterAll)
	}
	for _, name := range api.RunFilters {
		if !strings.Contains(f.Usage, string(name)) {
			t.Errorf("the flag's help must name %q — the four words are the whole interface: %q", name, f.Usage)
		}
	}
}

// TestRunListAndTheTabAgreeOnActive mirrors TestPRListAndTheTabAgreeOnActive.
func TestRunListAndTheTabAgreeOnActive(t *testing.T) {
	f, err := api.ParseRunFilter("active")
	if err != nil {
		t.Fatal(err)
	}
	got := api.FilterRuns(f, []api.Run{{ID: "a", Status: "queued"}, {ID: "b", Status: "passed"}})
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("--filter active gave %v, want the queued run alone", got)
	}
}

// NewRunCmd must expose exactly the operations the TUI reaches: list, info, output, cancel,
// priority — the CLI group and the tab describe the same run service, not two.
func TestNewRunCmdExposesEveryOperation(t *testing.T) {
	c := NewRunCmd()
	want := []string{"list", "info", "output", "cancel", "priority"}
	got := map[string]bool{}
	for _, sub := range c.Commands() {
		got[sub.Name()] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("`run` is missing the %q subcommand", name)
		}
	}
}
