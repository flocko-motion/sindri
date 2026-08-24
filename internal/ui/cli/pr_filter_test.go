package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestPRListFilterFlagDefaultsToActive mirrors TestTaskListFilterFlagDefaultsToActive (sd-4be9f8):
// the flag now defaults to "active", matching mail list and the TUI. --filter all recovers the
// whole history.
func TestPRListFilterFlagDefaultsToActive(t *testing.T) {
	f := prListCmd().Flags().Lookup("filter")
	if f == nil {
		t.Fatal("`pr list` must offer --filter; the TUI cycles the same set with `f`")
	}
	if f.DefValue != string(api.PRFilterActive) {
		t.Errorf("--filter defaults to %q, want %q — matching mail list and the TUI",
			f.DefValue, api.PRFilterActive)
	}
	for _, name := range api.PRFilters {
		if !strings.Contains(f.Usage, string(name)) {
			t.Errorf("the flag's help must name %q — the four words are the whole interface: %q", name, f.Usage)
		}
	}
}

// TestPRListLimitFlagDefaultsTo50 mirrors TestTaskListLimitFlagDefaultsTo50 (sd-4be9f8).
func TestPRListLimitFlagDefaultsTo50(t *testing.T) {
	f := prListCmd().Flags().Lookup("limit")
	if f == nil {
		t.Fatal("`pr list` must offer --limit")
	}
	if f.DefValue != "50" {
		t.Errorf("--limit defaults to %q, want 50", f.DefValue)
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
