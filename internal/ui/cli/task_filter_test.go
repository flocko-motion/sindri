package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestTaskListFilterFlagDefaultsToTodaysBehaviour: the TUI has had a four-way filter and the CLI
// none, which is a behaviour one front-end could not reach. The flag closes that — and defaults to
// "all", which is what the bare command has always printed, so no existing use changes.
func TestTaskListFilterFlagDefaultsToTodaysBehaviour(t *testing.T) {
	f := taskListCmd().Flags().Lookup("filter")
	if f == nil {
		t.Fatal("`task list` must offer --filter; the TUI cycles the same set with `f`")
	}
	if f.DefValue != string(api.FilterAll) {
		t.Errorf("--filter defaults to %q, want %q — the bare command must keep printing every task",
			f.DefValue, api.FilterAll)
	}
	for _, name := range api.TaskFilters {
		if !strings.Contains(f.Usage, string(name)) {
			t.Errorf("the flag's help must name %q — the four words are the whole interface: %q", name, f.Usage)
		}
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
