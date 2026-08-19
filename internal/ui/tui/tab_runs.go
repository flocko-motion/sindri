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
	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/table"
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

// runsNoteText says what a run IS and who queues one. Permanent rather than an empty state: the
// surprising part is the SERIAL queue, and "why is mine not starting" is asked when the list is
// full, so a line that vanished once there were rows would hide itself exactly when it earns its
// place. Wrapped, never truncated — a half-sentence explains nothing.
const runsNoteText = "A run is a command executed in a fresh container against a copy of a " +
	"workspace — ONE at a time across every repo. Press N to queue one; agents queue their own."

// runsNote is the note wrapped to width: one line on a wide terminal, two on a narrow one.
func (m model) runsNote(width int) []string {
	return wrapContent([]string{dimStyle.Render(runsNoteText)}, max(20, width))
}

// runsBody is the Runs tab: the permanent note, then the ordinary list/detail split beneath it,
// sized so the note costs rows rather than pushing them off the screen.
func (m model) runsBody() string {
	note := m.runsNote(m.w)
	list := m.runsList(m.leftWidth())
	if !m.showDetail() {
		list = m.runsList(m.w)
		return strings.Join(append(note, list), "\n")
	}
	dlines, dhl := m.wrappedDetail()
	right := pane(dlines, m.detail, m.detailWidth(), dhl)
	split := lipgloss.JoinHorizontal(lipgloss.Top, list, divider(m.runsPaneHeight()), right)
	return strings.Join(append(note, split), "\n")
}

// runsPaneHeight is what is left for the rows once the note has its lines. At least one, so a
// terminal too short for both still shows a run rather than only the sentence describing runs.
func (m model) runsPaneHeight() int {
	return max(1, m.bodyHeight()-len(m.runsNote(m.w)))
}

// runsList is the rows, or the empty state where the rows would be — "no runs" belongs in the list,
// in the shape the other panes use for emptiness, so a tab with nothing queued does not read broken.
func (m model) runsList(width int) string {
	rows := m.rows()
	if len(rows) == 0 {
		lines := make([]string, m.runsPaneHeight())
		lines[0] = padTrunc(dimStyle.Render("(no runs)"), width)
		for i := 1; i < len(lines); i++ {
			lines[i] = strings.Repeat(" ", width)
		}
		return strings.Join(lines, "\n")
	}
	return pane(rowTexts(rows), m.list, width, m.selRow())
}

// runRequester names who asked for a run. A user's reads "you", not the bare sentinel: the column
// is otherwise a list of agent names with one word in it that looks like another agent.
func runRequester(r api.Run) string {
	if api.RunFromUser(r) {
		return "you"
	}
	return r.Agent
}

// runsTable is the Runs list's columns, read by its header and every row alike.
var runsTable = table.Table{
	{Label: "repo", Width: 10, Clip: true},
	{Label: "run", Width: 14},
	{Label: "status", Width: 12},
	{Label: "age", Width: 4, Right: true},
	{Label: "queued by", Width: 10},
	{Label: "command"},
}

func (m model) runRows() []row {
	var out []row
	for _, r := range api.FilterRuns(m.runFilter, m.state.Runs) {
		if !m.inScope(r.Project) {
			continue
		}
		out = append(out, row{runsTable.Line(
			table.Cell{Text: m.repoName(r.Project), Style: m.repoStyle(r.Project).Render},
			table.Cell{Text: r.ID},
			table.Cell{Text: runStatusLabel(r)},
			table.Cell{Text: shortAge(r.CreatedAt)},
			table.Cell{Text: runRequester(r)},
			table.Cell{Text: r.Command},
		), r.ID})
	}
	return m.listing(runsTable, nil, out)
}

// runDetailLines is the full run detail for the right column / ENTER modal: its metadata, then
// its stored output (already capped by the hub — never re-capped here, or a truncation notice
// could read as this view's own).
func (m model) runDetailLines() []string { return itemTexts(m.runItems()) }

// runItems is the selected run's detail; who queued it and the task it was aimed at (if any) are
// cross-references, so a run can be chased back to the agent or task it belongs to.
func (m model) runItems() []metaItem {
	id := m.selID()
	if id == "" {
		return []metaItem{{text: dimStyle.Render("(no run)")}}
	}
	d := m.runDetail
	if d.Run.ID != id {
		return []metaItem{{text: id}, {text: dimStyle.Render("(loading…)")}}
	}
	r := d.Run
	by := metaItem{text: "by:       " + runRequester(r)}
	if !api.RunFromUser(r) {
		by.kind, by.value = "agent", r.Agent
	}
	items := []metaItem{
		{text: fmt.Sprintf("%s   [%s]", r.ID, runStatusLabel(r))},
		by,
		{text: "command:  " + r.Command},
		{text: "against:  " + api.RunTarget(r)},
	}
	if r.Task != "" {
		items = append(items, metaItem{text: "task:     " + r.Task, kind: "task", value: r.Task})
	}
	if r.Priority != "" {
		items = append(items, metaItem{text: "priority: " + r.Priority})
	}
	items = append(items, metaItem{text: "created:  " + r.CreatedAt})
	if r.StartedAt != "" {
		items = append(items, metaItem{text: "started:  " + r.StartedAt})
	}
	if r.FinishedAt != "" {
		items = append(items, metaItem{text: "finished: " + r.FinishedAt})
	}
	items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render("── output ──")})
	if d.Output == "" {
		items = append(items, metaItem{text: dimStyle.Render("(none yet)")})
	} else {
		for _, l := range strings.Split(strings.TrimRight(d.Output, "\n"), "\n") {
			items = append(items, metaItem{text: l})
		}
	}
	return items
}

// runActionable is the focusable subset of the run detail (who queued it, and its task).
func (m model) runActionable() []metaItem {
	var out []metaItem
	for _, it := range m.runItems() {
		if it.kind != "" {
			out = append(out, it)
		}
	}
	return out
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
