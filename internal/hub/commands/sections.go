// package: hub/commands / sections
// type:    logic (dashboard section registry)
// job:     the ordered dashboard tabs every UI renders and how each derives its
//          actionable badge count. The count reads from an injected Board — the hub
//          owns the cross-module snapshot (its BoardState) and satisfies this
//          interface, so the section registry lives here without importing the hub.
// limits:  the section list + count rule only; the data comes from the injected Board.
package commands

// Board is the snapshot a section's badge count reads from — injected by the hub,
// which assembles it across every module (its BoardState satisfies this interface).
// Reversing the dependency this way lets the section registry live outside the hub.
type Board interface {
	OpenTaskCount() int
	AgentCount() int
	OpenPRCount() int
	RepoCount() int
	ChatMemberCount() int
}

// Section is one dashboard tab: a key, a title, and its actionable badge count read
// from the board.
type Section struct {
	Key   string
	Title string
	Count func(Board) int
}

// Sections is the ordered set of dashboard sections — add one here and every UI picks
// it up.
var Sections = []Section{
	{"tasks", "Tasks", func(b Board) int { return b.OpenTaskCount() }},
	{"agents", "Agents", func(b Board) int { return b.AgentCount() }}, // whole roster — down agents are still agents
	{"prs", "PRs", func(b Board) int { return b.OpenPRCount() }},
	{"repos", "Repos", func(b Board) int { return b.RepoCount() }},
	{"chat", "Meeting", func(b Board) int { return b.ChatMemberCount() }},
}
