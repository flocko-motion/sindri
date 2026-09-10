// package: ui/cli / listlimit_test
// type:    logic (tests for the shared --limit machinery, sd-4be9f8)
// job:     limitNotice's wording, which end capHead/capTail each keep, and that every
// capped command offers --limit (with -n kept where it already existed).
// limits:  this behaviour only; each command's own filter defaults are pinned in its own
// <noun>_filter_test.go.
package cli

import (
	"strings"
	"testing"
)

// TestLimitNoticeSaysNothingWhenNothingWasCut: a limit that never bit must stay silent, the same
// as the existing filter notices do — a line that always prints trains the reader to ignore it.
func TestLimitNoticeSaysNothingWhenNothingWasCut(t *testing.T) {
	if got := limitNotice("task", 5, 5); got != "" {
		t.Errorf("nothing was cut, want no notice, got %q", got)
	}
}

// TestLimitNoticeNamesTheCountAndTheFlag: silent truncation would read as a complete answer, which
// for a listing is a wrong one (gitcmd.go's capLines states the same principle for a diff). The
// notice must say how many were cut and how to see the rest.
func TestLimitNoticeNamesTheCountAndTheFlag(t *testing.T) {
	got := limitNotice("run", 50, 62)
	for _, want := range []string{"50", "62", "run(s)", "--limit"} {
		if !strings.Contains(got, want) {
			t.Errorf("the notice should mention %q: %q", want, got)
		}
	}
}

// TestCapHeadKeepsTheFirstN pins which end capHead keeps: PR list and task list both order newest
// (or highest priority) first, so the kept prefix must be that end, not an arbitrary one a future
// re-sort could silently flip.
func TestCapHeadKeepsTheFirstN(t *testing.T) {
	kept, matched := capHead([]string{"a", "b", "c", "d"}, 2)
	if matched != 4 {
		t.Errorf("matched = %d, want 4", matched)
	}
	if strings.Join(kept, "") != "ab" {
		t.Errorf("capHead kept %v, want the first two", kept)
	}
}

// TestCapTailKeepsTheLastN pins which end capTail keeps: run list, meeting log and hub logs all
// order oldest first, so the kept suffix must be that end — the newest, not the earliest.
func TestCapTailKeepsTheLastN(t *testing.T) {
	kept, matched := capTail([]string{"a", "b", "c", "d"}, 2)
	if matched != 4 {
		t.Errorf("matched = %d, want 4", matched)
	}
	if strings.Join(kept, "") != "cd" {
		t.Errorf("capTail kept %v, want the last two", kept)
	}
}

// TestCapZeroOrUnderLimitKeepsEverything: 0 means unbounded, and a set no bigger than the limit is
// never touched — either way capHead/capTail must be a no-op, not an accidental empty slice.
func TestCapZeroOrUnderLimitKeepsEverything(t *testing.T) {
	for _, limit := range []int{0, 4, 5} {
		if kept, _ := capHead([]int{1, 2, 3, 4}, limit); len(kept) != 4 {
			t.Errorf("capHead(_, %d) kept %v, want all 4", limit, kept)
		}
		if kept, _ := capTail([]int{1, 2, 3, 4}, limit); len(kept) != 4 {
			t.Errorf("capTail(_, %d) kept %v, want all 4", limit, kept)
		}
	}
}

// TestEveryCappedListingOffersLimit (sd-4be9f8): one flag name across every list, defaulting to 50 —
// task list, PR list and run list are pinned individually in their own <noun>_filter_test.go; this
// covers the rest, including meeting log and hub logs where -n predates --limit and stays as its
// shorthand rather than breaking a working invocation.
func TestEveryCappedListingOffersLimit(t *testing.T) {
	if f := taskInfoCmd().Flags().Lookup("limit"); f == nil {
		t.Error("`task info` must offer --limit")
	} else if f.DefValue != "50" {
		t.Errorf("`task info`: --limit defaults to %q, want 50", f.DefValue)
	}

	if f := mailListCmd().Flags().Lookup("limit"); f == nil {
		t.Error("`mail list` must offer --limit")
	} else if f.DefValue != "50" {
		t.Errorf("`mail list`: --limit defaults to %q, want 50", f.DefValue)
	}

	if f := chatLogCmd().Flags().Lookup("limit"); f == nil {
		t.Error("`meeting log` must offer --limit")
	} else if f.DefValue != "50" {
		t.Errorf("`meeting log`: --limit defaults to %q, want 50", f.DefValue)
	}
	if f := chatLogCmd().Flags().ShorthandLookup("n"); f == nil || f.Name != "limit" {
		t.Error("`meeting log`: -n should stay as --limit's shorthand — the working invocation it had before")
	}

	if f := newHubLogsCmd().Flags().Lookup("limit"); f == nil {
		t.Error("`hub logs` must offer --limit")
	} else if f.DefValue != "50" {
		t.Errorf("`hub logs`: --limit defaults to %q, want 50", f.DefValue)
	}
	if f := newHubLogsCmd().Flags().ShorthandLookup("n"); f == nil || f.Name != "limit" {
		t.Error("`hub logs`: -n should stay as --limit's shorthand — the working invocation it had before")
	}
}
