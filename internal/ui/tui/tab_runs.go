// package: tui / runs
// type:    ui (Runs tab)
// job:     the Runs tab content — a list of scheduled commands with their state,
// agent, and queue position, plus cancel/reprioritise actions.
// limits:  renders run state and actions; the queue itself is the hub's
// (-> client / workflow/run.go).
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// runStatusLabel decorates a queued run's status with its queue position — "queued" alone
// never says where in line it is.
func runStatusLabel(r api.Run) string {
	if r.Status == "queued" && r.Position > 0 {
		return fmt.Sprintf("queued(#%d)", r.Position)
	}
	return r.Status
}

// runRequester names who asked for a run. A user's reads "you", not the bare sentinel: the column
// is otherwise a list of agent names with one word in it that looks like another agent.
func runRequester(r api.Run) string {
	if api.RunFromUser(r) {
		return "you"
	}
	return r.Agent
}

func (m model) runRows() []row {
	var out []row
	for _, r := range api.FilterRuns(m.runFilter, m.state.Runs) {
		if !m.inScope(r.Project) {
			continue
		}
		repo := m.repoStyle(r.Project).Render(fmt.Sprintf("%-10.10s", m.repoName(r.Project)))
		out = append(out, row{fmt.Sprintf("%s %-14s %-12s %4s %-10s %s",
			repo, r.ID, runStatusLabel(r), shortAge(r.CreatedAt), runRequester(r), r.Command), r.ID})
	}
	return out
}

// runDetailLines is the full run detail for the right column / ENTER modal: its metadata, then
// its stored output (already capped by the hub — never re-capped here, or a truncation notice
// could read as this view's own).
func (m model) runDetailLines() []string {
	id := m.selID()
	if id == "" {
		return []string{dimStyle.Render("(no run)")}
	}
	d := m.runDetail
	if d.Run.ID != id {
		return []string{id, dimStyle.Render("(loading…)")}
	}
	r := d.Run
	ls := []string{
		fmt.Sprintf("%s   [%s]   by %s", r.ID, runStatusLabel(r), runRequester(r)),
		"command: " + r.Command,
		"against: " + api.RunTarget(r),
	}
	if r.Priority != "" {
		ls = append(ls, "priority: "+r.Priority)
	}
	ls = append(ls, "created: "+r.CreatedAt)
	if r.StartedAt != "" {
		ls = append(ls, "started: "+r.StartedAt)
	}
	if r.FinishedAt != "" {
		ls = append(ls, "finished: "+r.FinishedAt)
	}
	ls = append(ls, "", dimStyle.Render("── output ──"))
	if d.Output == "" {
		ls = append(ls, dimStyle.Render("(none yet)"))
	} else {
		ls = append(ls, strings.Split(strings.TrimRight(d.Output, "\n"), "\n")...)
	}
	return ls
}

// openRunCancelChoice confirms withdrawing a queued or running run — "cancel" is both the run
// action and the modal's own back-out, so the options are worded to keep the two apart.
func (m *model) openRunCancelChoice(id string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "cancel " + id + "? it will not run",
		options: []string{"back", "cancel " + id},
		values:  []string{"back", "cancel"},
		apply: func(v string) tea.Cmd {
			if v != "cancel" {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.CancelRun(id) })
		},
	}
}

// openRunPriorityChoice reprioritises a queued run, reusing the same P-code vocabulary and
// labels the Tasks tab's priority picker uses.
func (m *model) openRunPriorityChoice(id string) {
	cl := m.cl
	vals := make([]string, len(theme.PriorityWords))
	for i, w := range theme.PriorityWords {
		vals[i] = theme.PriorityCode(w)
	}
	m.choice = choiceModalState{
		active: true, title: "priority for " + id,
		options: theme.PriorityWords, values: vals,
		apply: func(code string) tea.Cmd {
			return mutateThenRefresh(cl, func() error { return cl.ReprioritiseRun(id, code) })
		},
	}
}
