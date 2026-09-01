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
		Runs:   []api.Run{{ID: "r1", Status: "queued"}, {ID: "r2", Status: "running"}, {ID: "r3", Status: "passed"}},
		// Mail's badge is UNREAD, counted over the whole mailbox rather than the window on the board:
		// a mailbox is never pruned, so a total would only ever climb.
		Mail:           []api.Mail{{ID: 1, Agent: "x"}, {ID: 2, Agent: "x", ReadAt: "2026-08-13T10:00:00Z"}},
		MailTotal:      9,
		MailUnread:     4,
		MailUnreadUser: 2,
	}
	// non-closed; whole roster; open only (merged AND scrapped excluded); queued+running only; unread
	want := map[string]int{"tasks": 2, "agents": 2, "prs": 1, "runs": 2, "mail": 4}
	for _, s := range Sections {
		if got := s.Count(b); got != want[s.Key] {
			t.Errorf("%s count = %d, want %d", s.Key, got, want[s.Key])
		}
	}
}

// fakeBoard is a minimal Board for testing Resolved without a real BoardState.
type fakeBoard struct{}

func (fakeBoard) OpenTaskCount() int          { return 3 }
func (fakeBoard) AgentCount() int             { return 2 }
func (fakeBoard) OpenPRCount() int            { return 1 }
func (fakeBoard) OpenRunCount() int           { return 6 }
func (fakeBoard) RepoCount() int              { return 5 }
func (fakeBoard) ChatMemberCount() int        { return 4 }
func (fakeBoard) TasksNeedingUserCount() int  { return 2 }
func (fakeBoard) AgentsNeedingUserCount() int { return 1 }
func (fakeBoard) PRsNeedingUserCount() int    { return 3 }
func (fakeBoard) UnreadMailCount() int        { return 7 }
func (fakeBoard) UnreadUserMailCount() int    { return 2 }

// TestResolvedReadsEveryCount: Resolved is what actually crosses the wire — the
// registry's Count funcs can't — so each section's Key and Title must survive and
// its Count must come from calling the recipe against the given board, in order.
func TestResolvedReadsEveryCount(t *testing.T) {
	got := Resolved(fakeBoard{})
	if len(got) != len(Sections) {
		t.Fatalf("got %d resolved sections, want %d", len(got), len(Sections))
	}
	want := map[string]int{"tasks": 3, "agents": 2, "prs": 1, "runs": 6, "repos": 5, "chat": 4, "mail": 7}
	// Repos, Meeting and Runs have no Attention recipe: they resolve to 0 rather than panicking on a
	// nil call, which is what a section holding nothing a human can wait on looks like. Mail HAS one,
	// counting only what is addressed to the user — the rest of the mailbox is the agents' backlog.
	wantAttention := map[string]int{"tasks": 2, "agents": 1, "prs": 3, "mail": 2}
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

// TestAttentionCountsWhatOnlyTheUserCanMove pins all three markers against a real board: the Tasks
// section counts what the approval gate holds, Agents counts agents whose state needs a human, and
// PRs counts what waits on a merge or on a review nobody is left to give. A plain idle agent is in
// none of them — it is waiting for work, which is not a fault.
func TestAttentionCountsWhatOnlyTheUserCanMove(t *testing.T) {
	b := api.BoardState{
		Tasks: []api.Task{
			{ID: "a", Status: "open", Approval: "pending", Priority: "P1"},
			{ID: "b", Status: "open", Priority: "P1"},
			// Unrated: no priority anywhere above it, so no worker can be handed it either — the
			// other gate, and counted the same way (-> api.TaskNeedsUser).
			{ID: "c", Status: "open"},
		},
		Agents: []api.AgentView{
			{Name: "blocked", Status: api.StatusBlocked, NeedsUser: true},
			{Name: "signed-out", Status: api.StatusSignedOut, NeedsUser: true},
			{Name: "escalated", Status: api.StatusEscalated, NeedsUser: true},
			{Name: "stalled", Status: api.StatusStalled, NeedsUser: true},
			{Name: "idle", Status: "idle"},
			{Name: "working", Status: "working"},
			// Retired against a status that counts: it keeps running, so this is what winding one
			// down actually looks like a few hours later.
			{Name: "retired", Status: api.StatusStalled, Retired: true},
			// A healthy reviewer, in a DIFFERENT repo from the PRs below: it is handed none of
			// them, so it leaves their queue exactly as stranded as an empty roster would.
			{Project: "other", Name: "fili", Role: "reviewer", Status: "idle"},
		},
		// No reviewer in project p, so its open PR is stranded; the approved one waits on the merge
		// whatever is running, and the merged and rejected ones wait on nobody.
		PRs: []api.PR{
			{Project: "p", ID: "pr-open", Status: "open"},
			{Project: "p", ID: "pr-approved", Status: "approved"},
			{Project: "p", ID: "pr-merged", Status: "merged"},
			{Project: "p", ID: "pr-rejected", Status: "rejected"},
			{Project: "p", ID: "pr-merge-failed", Status: "merge-failed"}, // a restart caught it mid-merge
		},
	}
	want := map[string]int{"tasks": 2, "agents": 4, "prs": 3}
	for _, s := range Resolved(b) {
		if s.Attention != want[s.Key] {
			t.Errorf("%s attention = %d, want %d", s.Key, s.Attention, want[s.Key])
		}
	}
}
