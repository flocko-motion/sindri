package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestPRListFilterFlagDefaultsToTodaysBehaviour mirrors TestTaskListFilterFlagDefaultsToTodaysBehaviour:
// the flag defaults to "all", so a bare `pr list` still prints every PR as it always has.
func TestPRListFilterFlagDefaultsToTodaysBehaviour(t *testing.T) {
	f := prListCmd().Flags().Lookup("filter")
	if f == nil {
		t.Fatal("`pr list` must offer --filter; the TUI cycles the same set with `f`")
	}
	if f.DefValue != string(api.PRFilterAll) {
		t.Errorf("--filter defaults to %q, want %q — the bare command must keep printing every PR",
			f.DefValue, api.PRFilterAll)
	}
	for _, name := range api.PRFilters {
		if !strings.Contains(f.Usage, string(name)) {
			t.Errorf("the flag's help must name %q — the four words are the whole interface: %q", name, f.Usage)
		}
	}
}

// TestPRListAndTheTabAgreeOnActive mirrors TestTaskListAndTheTabAgreeOnActive: both front-ends
// call api.MatchesPRFilter, so the same board gives the same rows either way.
func TestPRListAndTheTabAgreeOnActive(t *testing.T) {
	f, err := api.ParsePRFilter("active")
	if err != nil {
		t.Fatal(err)
	}
	got := api.FilterPRs(f, []api.PR{{ID: "a", Status: "open"}, {ID: "b", Status: "merged"}})
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("--filter active gave %v, want the open PR alone", got)
	}
}
