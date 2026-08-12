// package: tui / scrap choice
// type:    ui (Tasks tab confirm modal)
// job:     the scrap (discard) confirm for the selected task: it measures what the discard
// can reach — the task's open PR, the tasks under it, their PRs — and offers one option per
// shape, then hands the chosen one to the hub.
// limits:  labels and key-to-call plumbing only; the subtree walk is the hub's
// (-> api.Descendants) and the cascade itself is ScrapTask's.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// attachedOpenPR is the id of a non-terminal PR for the task, or "". A merged or scrapped one is
// already off the board, so it is never offered for scrapping.
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

// measureScrapReach reads the reach off the current board. Subtasks count whatever their status —
// a done one is still left dangling by a scrapped parent, so the cascade takes it.
func (m model) measureScrapReach(id string) scrapReach {
	r := scrapReach{pr: m.attachedOpenPR(id)}
	if r.pr != "" {
		r.prs++
	}
	for _, d := range api.Descendants(m.state.Tasks, id) {
		r.kids++
		if m.attachedOpenPR(d.ID) != "" {
			r.prs++
		}
	}
	return r
}

// openScrapChoice gates a scrap behind yes/no, since a GitHub issue delete is permanent. Everything
// the discard could reach becomes its own option, so the wider scrap is never implied: the open PR,
// the tasks under it — which a task-only scrap strands as roots — and their PRs in turn.
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
		kids := theme.Plural(r.kids, "subtask", "subtasks")
		add("scrap task + "+kids, "tree")
		if r.prs > 0 {
			add("scrap task + "+kids+" + "+theme.Plural(r.prs, "PR", "PRs"), "treepr")
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
		has = append(has, theme.Plural(r.kids, "subtask", "subtasks"))
	}
	if len(has) == 0 {
		return "(discard — td delete / openspec remove / issue delete)"
	}
	return "(has " + strings.Join(has, ", ") + ")"
}

// scrapTaskCmd runs the chosen scrap through the hub — the task, optionally its
// subtree, optionally the open PRs of what goes — and refreshes the board once.
func scrapTaskCmd(cl *client.HTTP, id string, subtree, withPRs bool) tea.Cmd {
	scrap := func(string) error { return cl.ScrapTask(id, subtree, withPRs) }
	return finishTaskCmd(cl, scrap, id, "", false)
}
