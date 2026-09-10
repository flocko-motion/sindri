package pr

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/machine"
)

// TestEveryConditionIsWatched: a question nothing asks is a rule nobody reads.
func TestEveryConditionIsWatched(t *testing.T) {
	watched := map[string]bool{}
	for _, s := range Flow {
		for _, e := range s.Events {
			watched[e.On.EventName()] = true
		}
	}
	for _, c := range Conditions {
		if !watched[c.Name] {
			t.Errorf("condition %q is declared but no state watches it", c.Name)
		}
	}
}

// TestNothingMergesWithoutAHuman is the one hard gate in sindri, pinned as a property of the map:
// the approved state runs no action and declares no condition that lands it. A human types `merge`,
// and only a human ever does — a future edit that gives this state an action would be the whole
// safety model going quietly.
func TestNothingMergesWithoutAHuman(t *testing.T) {
	for _, s := range Flow {
		if s.Name != Approved {
			continue
		}
		if s.Action != nil {
			t.Fatalf("the approved state runs %q — nothing in sindri may merge on its own", s.Action.Name)
		}
		for _, e := range s.Events {
			if e.To == Merging {
				t.Errorf("approved leads to merging on %q; only a human verb may start a merge", e.On.EventName())
			}
		}
	}
}

// TestAClosedTaskScrapsItsPullRequest: a PR against a task that has closed can never land, and
// leaving it open made every reader carry the exception — hepti showed one on a task closed a week
// earlier while the hold rule said otherwise.
func TestAClosedTaskScrapsItsPullRequest(t *testing.T) {
	live := World{Exists: true, TaskOpen: false}
	for _, from := range []string{Filed, Gating, Reviewing, Approved, Rejected} {
		got := from
		for _, s := range Flow {
			if s.Name != from {
				continue
			}
			for _, e := range s.Events {
				if c, ok := e.On.(Condition); ok && c.Holds(live) {
					got = e.To
					break
				}
			}
		}
		if got != Scrapped {
			t.Errorf("%s over a closed task -> %s, want %s", from, got, Scrapped)
		}
	}
}

// TestAMergeThatDiedIsNotLeftReadingInFlight: a hub that died mid-merge leaves nobody knowing
// whether the base carries it. "merging" would read as in-flight for ever; the orphan exit asks for
// a human to look, which is what the boot-time reconcile did by hand.
func TestAMergeThatDiedIsNotLeftReadingInFlight(t *testing.T) {
	for _, s := range Flow {
		if s.Name != Merging {
			continue
		}
		for _, e := range s.Events {
			if _, ok := e.On.(machine.Orphaned); ok && e.To == Stuck {
				return
			}
		}
		t.Errorf("merging declares no orphan exit — a hub that dies mid-merge would strand it")
	}
}
