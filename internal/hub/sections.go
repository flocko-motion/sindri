// package: hub / sections
// type:    logic (dashboard badge counts)
// job:     the single source of truth for the dashboard's sections and their
//          actionable badge counts, derived from the whole board snapshot. UIs render
//          these; they never re-derive the counts.
// limits:  count derivation only — needs the full BoardState, so it lives hub-side.
//          The task label/ordering helpers moved to hub/task (view.go).
package hub

import (
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// Section is one dashboard tab: a key, a title, and its actionable badge count
// derived from board state.
type Section struct {
	Key   string
	Title string
	Count func(BoardState) int
}

// Sections is the ordered set of dashboard sections — add one here and every UI
// picks it up.
var Sections = []Section{
	{"tasks", "Tasks", func(b BoardState) int { return countTasks(b.Tasks, task.Open) }},
	{"agents", "Agents", func(b BoardState) int { return len(b.Agents) }}, // the whole roster — down agents are still agents
	{"prs", "PRs", func(b BoardState) int { return countPRs(b.PRs, prNotMerged) }},
	{"repos", "Repos", func(b BoardState) int { return len(b.Projects) }},   // every repo the hub tracks
	{"chat", "Chat", func(b BoardState) int { return len(b.Chat.Members) }}, // agents in the user's chatroom
}

func prNotMerged(p store.PR) bool { return p.Status != "merged" }

func countTasks(ts []store.Task, pred func(store.Task) bool) (n int) {
	for _, t := range ts {
		if pred(t) {
			n++
		}
	}
	return
}
func countPRs(ps []store.PR, pred func(store.PR) bool) (n int) {
	for _, p := range ps {
		if pred(p) {
			n++
		}
	}
	return
}
