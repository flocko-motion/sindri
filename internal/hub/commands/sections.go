// package: hub/commands / sections
// type:    logic (dashboard section registry)
// job:     the ordered dashboard tabs every UI renders, each with its badge count and
// how many of those rows wait on the user. Both read from an injected Board —
// the hub owns that snapshot (its BoardState) and satisfies this interface, so
// the registry lives here without importing the hub.
// limits:  the section list and its two count rules; the data comes from the Board.
// A func can't cross the wire, so Resolved is what a client actually receives.
package commands

import "github.com/flo-at/sindri/internal/api"

// Board is the snapshot a section's badge count reads from — injected by the hub,
// which assembles it across every module (its BoardState satisfies this interface).
// Reversing the dependency this way lets the section registry live outside the hub.
type Board interface {
	OpenTaskCount() int
	AgentCount() int
	OpenPRCount() int
	OpenRunCount() int
	RepoCount() int
	ChatMemberCount() int
	TasksNeedingUserCount() int
	AgentsNeedingUserCount() int
	PRsNeedingUserCount() int
	UnreadMailCount() int
}

// Section is one dashboard tab: a key, a title, its actionable badge count read from the board,
// and how many of those rows are waiting on the user.
type Section struct {
	Key   string
	Title string
	Count func(Board) int
	// Attention counts the rows only the user can move on — the "(N!)" marker beside the count.
	// Every marker is a line here rather than a case in each view, which is what keeps the three
	// of them one behaviour. nil where a section holds nothing that can wait on a human.
	Attention func(Board) int
}

// Sections is the ordered set of dashboard sections — add one here and every UI picks
// it up.
var Sections = []Section{
	{
		Key: "tasks", Title: "Tasks",
		Count: func(b Board) int { return b.OpenTaskCount() },
		// Work behind either gate — unapproved or unrated — is hidden from every worker, so a
		// backlog of it reads as plenty to do beside an idle agent.
		Attention: func(b Board) int { return b.TasksNeedingUserCount() },
	},
	{
		Key: "agents", Title: "Agents",
		Count: func(b Board) int { return b.AgentCount() }, // whole roster — down agents are still agents
		// Blocked, signed out, full or stalled: each looks alive, holds its task and makes no
		// progress, and none of them clears without a human.
		Attention: func(b Board) int { return b.AgentsNeedingUserCount() },
	},
	{
		Key: "prs", Title: "PRs",
		Count: func(b Board) int { return b.OpenPRCount() },
		// Waiting on a merge, on a merge that died in flight, or on a review nobody is left to
		// give — an interim PR being user-gated by design (-> api.PRWaitReason).
		Attention: func(b Board) int { return b.PRsNeedingUserCount() },
	},
	// No Attention: a run's failure is the scheduling agent's to see and rerun, not a verdict a
	// human owes — that changes once the submit gate routes through this queue (sd-cf630b), which
	// should revisit this rather than inherit it by default.
	{Key: "runs", Title: "Runs", Count: func(b Board) int { return b.OpenRunCount() }},
	{Key: "repos", Title: "Repos", Count: func(b Board) int { return b.RepoCount() }},
	{Key: "chat", Title: "Meeting", Count: func(b Board) int { return b.ChatMemberCount() }},
	{
		Key: "mail", Title: "Mail",
		// Unread, not the whole history: the mailbox never shrinks, so a total would climb for ever
		// and stop meaning anything, while unread is the one number that can go back to zero.
		Count: func(b Board) int { return b.UnreadMailCount() },
		// No Attention, deliberately: unread mail is the AGENT's backlog, not the user's work, and
		// nothing here waits on a human. An agent that has stopped reading is worth surfacing, and
		// that belongs on the agent's own row (-> sd-7b317b) rather than as a marker asking the user
		// to do something no verb of theirs can do.
	},
}

// Resolved reads every section's counts against b and returns the wire shape: the
// recipes (Count, Attention) stay here, only the numbers they produce cross.
func Resolved(b Board) []api.Section {
	out := make([]api.Section, len(Sections))
	for i, s := range Sections {
		out[i] = api.Section{Key: s.Key, Title: s.Title, Count: s.Count(b)}
		if s.Attention != nil {
			out[i].Attention = s.Attention(b)
		}
	}
	return out
}
