package task

import "testing"

// TestTheMapRepairsEveryStaleClaimTheSweepDid pins the rules the repair sweep used to apply by hand
// as the conditions they became. Each case was a real stale row: a task reading in_progress with no
// agent behind it, one reading in_review with nothing out, a merge that finished work nobody closed,
// and a parent closed over children still open — the incident all of this traces back to.
func TestTheMapRepairsEveryStaleClaimTheSweepDid(t *testing.T) {
	for _, c := range []struct {
		what  string
		from  string
		world World
		want  string
	}{
		{"in_progress with nobody holding it", Held, World{Exists: true}, Open},
		{"in_progress its holder still holds", Held, World{Exists: true, Holder: "dain"}, Held},
		{"in_review with nothing out", Reviewing, World{Exists: true, Holder: "dain"}, Held},
		{"in_review with a live PR", Reviewing, World{Exists: true, ActivePR: true, Holder: "dain"}, Reviewing},
		{"open once its work landed", Open, World{Exists: true, Landed: true}, Closed},
		{"landed but children remain", Open, World{Exists: true, Landed: true, OpenChildren: true}, Open},
		{"closed over open children", Closed, World{Exists: true, OpenChildren: true}, Open},
		{"closed with nothing beneath", Closed, World{Exists: true}, Closed},
		{"proposed until the user rules", Proposed, World{Exists: true, Pending: true}, Proposed},
		{"proposed once approved", Proposed, World{Exists: true}, Open},
		{"proposed and refused", Proposed, World{Exists: true, Refused: true}, Closed},
		{"gone from its source", Held, World{}, Closed},
	} {
		got := c.from
		for _, s := range Flow {
			if s.Name != c.from {
				continue
			}
			for _, e := range s.Events {
				cond, ok := e.On.(Condition)
				if ok && cond.Holds(c.world) {
					got = e.To
					break
				}
			}
		}
		if got != c.want {
			t.Errorf("%s: %s -> %s, want %s", c.what, c.from, got, c.want)
		}
	}
}

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
