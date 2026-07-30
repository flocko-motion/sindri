// package: tui / scrap choice
// type:    ui (Tasks tab confirm modal)
// job:     the scrap (discard) confirm for the selected task — it measures what the
//          discard can reach (the task's open PR, the tasks under it, their PRs) and
//          offers one option per shape, then hands the chosen shape to the hub.
// limits:  labels and key-to-call plumbing only; the subtree walk is the hub's
//          (-> hub.Descendants) and the cascade itself is ScrapTask's.
package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/client"
)

// attachedOpenPR returns the id of an open (non-terminal) PR for task id in the active
// repo, or "" if none. A merged/scrapped PR is already off the board, so it's never
// offered for scrapping.
func (m model) attachedOpenPR(taskID string) string {
	_, tag := m.currentRepo()
	for _, p := range m.state.PRs {
		if p.Project == tag && p.Task == taskID && p.Status != "merged" && p.Status != "scrapped" {
			return p.ID
		}
	}
	return ""
}

// scrapReach is what scrapping the selected task can take with it: its own open PR,
// how many tasks sit under it, and how many open PRs the whole set has.
type scrapReach struct {
	pr   string
	kids int
	prs  int
}

// measureScrapReach reads the reach off the current board. Subtasks count whatever their
// status: a done one is still left dangling by a scrapped parent, so the cascade takes it
// and the label has to say so.
func (m model) measureScrapReach(id string) scrapReach {
	r := scrapReach{pr: m.attachedOpenPR(id)}
	if r.pr != "" {
		r.prs++
	}
	for _, d := range hub.Descendants(m.state.Tasks, id) {
		r.kids++
		if m.attachedOpenPR(d.ID) != "" {
			r.prs++
		}
	}
	return r
}

// openScrapChoice confirms scrapping (deleting) the selected task — destructive (a
// GitHub issue delete is permanent), so it's gated behind a yes/no. The hub dispatches
// to the backend (td delete / openspec change removal / issue delete).
//
// Anything the discard could reach past the task itself becomes its own option, so the
// wider scrap is never implied: its open PR (branch deleted, working agent stopped),
// the tasks under it — which a task-only scrap would leave behind as roots of work
// nobody asked for — and the open PRs of that whole set.
func (m *model) openScrapChoice(id string) {
	cl := m.cl
	r := m.measureScrapReach(id)
	opts, vals := []string{"cancel"}, []string{"cancel"}
	add := func(label, value string) {
		opts, vals = append(opts, label), append(vals, value)
	}
	if r.pr == "" && r.kids == 0 {
		add("scrap", "task") // nothing else in play — a plain confirm
	} else {
		add("scrap task only", "task")
	}
	if r.pr != "" {
		add("scrap task + PR "+r.pr, "taskpr")
	}
	if r.kids > 0 {
		kids := plural(r.kids, "subtask", "subtasks")
		add("scrap task + "+kids, "tree")
		if r.prs > 0 {
			add("scrap task + "+kids+" + "+plural(r.prs, "PR", "PRs"), "treepr")
		}
	}
	m.choice = choiceModalState{
		active: true, title: "scrap " + id + "?  " + scrapTitleTail(r),
		options: opts, values: vals,
		apply: func(v string) tea.Cmd {
			if v == "cancel" {
				return nil
			}
			subtree, withPRs := v == "tree" || v == "treepr", v == "taskpr" || v == "treepr"
			return taskOpTrigger(id, "deleting", scrapTaskCmd(cl, id, subtree, withPRs))
		},
	}
}

// scrapTitleTail names what the task still has attached — the reason the modal offers
// more than a yes/no — or what a bare scrap does when it has nothing.
func scrapTitleTail(r scrapReach) string {
	var has []string
	if r.pr != "" {
		has = append(has, "open PR "+r.pr)
	}
	if r.kids > 0 {
		has = append(has, plural(r.kids, "subtask", "subtasks"))
	}
	if len(has) == 0 {
		return "(discard — td delete / openspec remove / issue delete)"
	}
	return "(has " + strings.Join(has, ", ") + ")"
}

// plural renders a counted noun ("1 subtask", "3 subtasks") for the labels.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// scrapTaskCmd runs the chosen scrap through the hub — the task, optionally its
// subtree, optionally the open PRs of what goes — and refreshes the board once.
func scrapTaskCmd(cl *client.HTTP, id string, subtree, withPRs bool) tea.Cmd {
	scrap := func(string) error { return cl.ScrapTask(id, subtree, withPRs) }
	return finishTaskCmd(cl, scrap, id, "", false)
}
