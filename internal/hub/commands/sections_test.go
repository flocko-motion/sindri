package commands

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestSectionCounts pins each section's Count against a real api.BoardState (not fakeBoard):
// this is the shape a live board actually has, so a mismatch here would be BoardState's own
// count methods disagreeing with what the registry expects of them.
func TestSectionCounts(t *testing.T) {
	b := api.BoardState{
		Tasks: []api.Task{
			{ID: "a", Status: "open"}, {ID: "b", Status: "in_progress"},
			{ID: "c", Status: "closed"}, {ID: "d", Status: "merged"},
		},
		Agents: []api.AgentView{{Name: "x", Status: "idle"}, {Name: "y", Status: "down"}},
		PRs:    []api.PR{{ID: "p1", Status: "open"}, {ID: "p2", Status: "merged"}, {ID: "p3", Status: "scrapped"}},
	}
	want := map[string]int{"tasks": 2, "agents": 2, "prs": 1} // non-closed; whole roster; open only (merged AND scrapped excluded)
	for _, s := range Sections {
		if got := s.Count(b); got != want[s.Key] {
			t.Errorf("%s count = %d, want %d", s.Key, got, want[s.Key])
		}
	}
}

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
