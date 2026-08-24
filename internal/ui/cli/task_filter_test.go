package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestTaskListFilterFlagDefaultsToActive (sd-4be9f8): the TUI opens on "active" and stays readable;
// the CLI offered the same filter but shipped "all", which is how a bare `task list` grew to 264
// lines. --filter all still recovers the whole backlog.
func TestTaskListFilterFlagDefaultsToActive(t *testing.T) {
	f := taskListCmd().Flags().Lookup("filter")
	if f == nil {
		t.Fatal("`task list` must offer --filter; the TUI cycles the same set with `f`")
	}
	if f.DefValue != string(api.FilterActive) {
		t.Errorf("--filter defaults to %q, want %q — matching mail list and the TUI",
			f.DefValue, api.FilterActive)
	}
	for _, name := range api.TaskFilters {
		if !strings.Contains(f.Usage, string(name)) {
			t.Errorf("the flag's help must name %q — the four words are the whole interface: %q", name, f.Usage)
		}
	}
}

// TestTaskListLimitFlagDefaultsTo50 (sd-4be9f8): active alone still grows without bound as the
// fleet grows, so the bound needs a count too.
func TestTaskListLimitFlagDefaultsTo50(t *testing.T) {
	f := taskListCmd().Flags().Lookup("limit")
	if f == nil {
		t.Fatal("`task list` must offer --limit")
	}
	if f.DefValue != "50" {
		t.Errorf("--limit defaults to %q, want 50", f.DefValue)
	}
}

// TestTaskListAndTheTabAgreeOnActive: both front-ends call api.MatchesFilter, so this pins the
// consequence rather than the wiring — the same backlog gives the same rows either way.
func TestTaskListAndTheTabAgreeOnActive(t *testing.T) {
	f, err := api.ParseTaskFilter("active")
	if err != nil {
		t.Fatal(err)
	}
	got := api.FilterTasks(f, []api.Task{{ID: "a", Status: "open"}, {ID: "b", Status: "closed"}})
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("--filter active gave %v, want the open task alone", got)
	}
}
