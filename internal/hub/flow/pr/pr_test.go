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
// the approved state runs no action, and the ONE thing that leads it into merging is a recorded
// merge intent — a fact only a human's `merge` writes. A future edit that gave this state an action,
// or a second way in, would be the whole safety model going quietly.
func TestNothingMergesWithoutAHuman(t *testing.T) {
	for _, s := range Flow {
		if s.Name != Approved {
			continue
		}
		if s.Action != nil {
			t.Fatalf("the approved state runs %q — nothing in sindri may merge on its own", s.Action.Name)
		}
		var ways []string
		for _, e := range s.Events {
			if e.To == Merging {
				ways = append(ways, e.On.EventName())
			}
		}
		if len(ways) != 1 || ways[0] != mergeAsked.Name {
			t.Errorf("approved leads to merging on %v; the only way in is a human's recorded intent (%q)",
				ways, mergeAsked.Name)
		}
	}
	// And nothing but a human's verb writes that intent: the merging state's own action takes the
	// request back, and no other caller sets it.
	if !mergeAsked.Holds(World{MergeAsked: true}) || mergeAsked.Holds(World{}) {
		t.Error("the merge intent must be exactly the recorded request and nothing else")
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
