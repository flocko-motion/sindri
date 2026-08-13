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

func (fakeBoard) OpenTaskCount() int             { return 3 }
func (fakeBoard) AgentCount() int                { return 2 }
func (fakeBoard) OpenPRCount() int               { return 1 }
func (fakeBoard) RepoCount() int                 { return 5 }
func (fakeBoard) ChatMemberCount() int           { return 4 }
func (fakeBoard) TasksAwaitingVerdictCount() int { return 2 }
func (fakeBoard) AgentsNeedingUserCount() int    { return 1 }

// TestResolvedReadsEveryCount: Resolved is what actually crosses the wire — the
// registry's Count funcs can't — so each section's Key and Title must survive and
// its Count must come from calling the recipe against the given board, in order.
func TestResolvedReadsEveryCount(t *testing.T) {
	got := Resolved(fakeBoard{})
	if len(got) != len(Sections) {
		t.Fatalf("got %d resolved sections, want %d", len(got), len(Sections))
	}
	want := map[string]int{"tasks": 3, "agents": 2, "prs": 1, "repos": 5, "chat": 4}
	// A section with no Attention recipe resolves to 0 rather than panicking on a nil call — that is
	// what a section holding nothing a human can wait on looks like.
	wantAttention := map[string]int{"tasks": 2, "agents": 1}
	for i, s := range got {
		if s.Key != Sections[i].Key || s.Title != Sections[i].Title {
			t.Errorf("resolved[%d] = %+v, want key/title from Sections[%d] = %+v", i, s, i, Sections[i])
		}
		if want[s.Key] != s.Count {
			t.Errorf("resolved %q count = %d, want %d", s.Key, s.Count, want[s.Key])
		}
		if wantAttention[s.Key] != s.Attention {
			t.Errorf("resolved %q attention = %d, want %d", s.Key, s.Attention, wantAttention[s.Key])
		}
	}
}

// TestAttentionCountsWhatOnlyTheUserCanMove pins both markers against a real board: the Tasks
// section counts what the approval gate holds, the Agents section counts agents whose state needs
// a human. A plain idle agent is in neither — it is waiting for work, which is not a fault.
func TestAttentionCountsWhatOnlyTheUserCanMove(t *testing.T) {
	b := api.BoardState{
		Tasks: []api.Task{
			{ID: "a", Status: "open", Approval: "pending"},
			{ID: "b", Status: "open"},
		},
		Agents: []api.AgentView{
			{Name: "blocked", Status: api.StatusBlocked},
			{Name: "signed-out", Status: api.StatusSignedOut},
			{Name: "full", Status: api.StatusFull},
			{Name: "stalled", Status: api.StatusStalled},
			{Name: "idle", Status: "idle"},
			{Name: "working", Status: "working"},
			// Retired against a status that counts: it keeps running, so this is what winding one
			// down actually looks like a few hours later.
			{Name: "retired", Status: api.StatusFull, Retired: true},
		},
	}
	want := map[string]int{"tasks": 1, "agents": 4}
	for _, s := range Resolved(b) {
		if s.Attention != want[s.Key] {
			t.Errorf("%s attention = %d, want %d", s.Key, s.Attention, want[s.Key])
		}
	}
}
