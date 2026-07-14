// package: hub / sections
// type:    logic (board badge counts)
// job:     BoardState's actionable badge counts — the numbers the dashboard sections
//          (hub/commands) render. BoardState satisfies commands.Board, so the section
//          registry lives outside the hub while the counting (which needs the whole
//          cross-module snapshot) stays here.
// limits:  count derivation only; the section list + titles live in hub/commands.
package hub

import (
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// These make BoardState satisfy commands.Board — the badge counts the dashboard
// sections read.

// OpenTaskCount is the number of not-done tasks in the selected project.
func (b BoardState) OpenTaskCount() int { return countTasks(b.Tasks, task.Open) }

// AgentCount is the whole roster size (down agents are still agents).
func (b BoardState) AgentCount() int { return len(b.Agents) }

// OpenPRCount is the number of not-yet-merged PRs across the fleet.
func (b BoardState) OpenPRCount() int { return countPRs(b.PRs, prNotMerged) }

// RepoCount is the number of repos the hub tracks.
func (b BoardState) RepoCount() int { return len(b.Projects) }

// ChatMemberCount is the number of agents in the user's chatroom.
func (b BoardState) ChatMemberCount() int { return len(b.Chat.Members) }

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
