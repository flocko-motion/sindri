package commands

import "testing"

// fakeBoard is a minimal Board for testing Resolved without a real BoardState.
type fakeBoard struct{}

func (fakeBoard) OpenTaskCount() int   { return 3 }
func (fakeBoard) AgentCount() int      { return 2 }
func (fakeBoard) OpenPRCount() int     { return 1 }
func (fakeBoard) RepoCount() int       { return 5 }
func (fakeBoard) ChatMemberCount() int { return 4 }

// TestResolvedReadsEveryCount: Resolved is what actually crosses the wire — the
// registry's Count funcs can't — so each section's Key and Title must survive and
// its Count must come from calling the recipe against the given board, in order.
func TestResolvedReadsEveryCount(t *testing.T) {
	got := Resolved(fakeBoard{})
	if len(got) != len(Sections) {
		t.Fatalf("got %d resolved sections, want %d", len(got), len(Sections))
	}
	want := map[string]int{"tasks": 3, "agents": 2, "prs": 1, "repos": 5, "chat": 4}
	for i, s := range got {
		if s.Key != Sections[i].Key || s.Title != Sections[i].Title {
			t.Errorf("resolved[%d] = %+v, want key/title from Sections[%d] = %+v", i, s, i, Sections[i])
		}
		if want[s.Key] != s.Count {
			t.Errorf("resolved %q count = %d, want %d", s.Key, s.Count, want[s.Key])
		}
	}
}
