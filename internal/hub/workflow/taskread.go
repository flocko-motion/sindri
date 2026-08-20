// package: hub/workflow / taskread
// type:    logic (the agent's read surface over the backlog)
// job:     what `task` answers — the caller's own work, one task in full, and the
// backlog listed as a tree: which of it a role may read, the filter the
// listing applies, and the summary that says what the filter hid.
// limits:  reads and renders to a writer; creating and editing tasks is planner.go's,
// and the filter predicate itself is shared with both front-ends (-> api).
package workflow

import (
	"fmt"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// childIDs are the ids of the tasks parented by id, in listing order.
func childIDs(tasks []store.Task, id string) []string {
	var out []string
	for _, t := range tasks {
		if t.ParentID == id {
			out = append(out, t.ID)
		}
	}
	return out
}

// taskListUsage is the one description of `task list`'s surface: shown for a bad argument, and
// advertised through TaskHelp, so the verb list and the verb's own refusal say the same thing.
var taskListUsage = "usage: task list [--filter " + api.TaskFilterNames() + "]\n" +
	"  Defaults to `active`: open work plus whatever changed in the last " + api.ActiveWindow.String() + ". A backlog is\n" +
	"  mostly closed history, and the whole of it is a hundred rows of what nobody can act on.\n" +
	"  Every listing closes by saying what it showed and how big the backlog is, so a narrow\n" +
	"  default can never read as an empty one. `--filter all` is the whole thing."

// TaskHelp is what the registry advertises for `task`.
const TaskHelp = "read your work: `task` (your own task or package; a planner or coauthor: the whole backlog), " +
	"`task <id>` (one task in full — description, parent, children), " +
	"`task list [--filter open|closed|all|active]` (the backlog, indented by tree; active by default)"

// parseTaskListFlags reads `task list`'s only flag, in both spellings. An unknown flag or value is
// REFUSED, not ignored: identical output under every filter reads as a filter that does not work.
// Stricter than api.MatchesFilter on purpose — its leniency answers a question that was understood,
// where an unknown argument does not answer at all.
func parseTaskListFlags(args []string) (api.TaskFilter, error) {
	f := api.FilterActive
	for i := 0; i < len(args); i++ {
		name, inline, hasInline := strings.Cut(args[i], "=")
		switch name {
		case "--filter", "-f":
			val := inline
			if !hasInline {
				if i+1 >= len(args) {
					return f, fmt.Errorf("%s needs a value", name)
				}
				i++
				val = args[i]
			}
			parsed, err := api.ParseTaskFilter(val)
			if err != nil {
				return f, err
			}
			f = parsed
		default:
			return f, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	return f, nil
}

// CmdTasks is the read surface over the backlog, scoped to the caller's job: a planner or
// coauthor shapes all of it, a worker sees only the package it holds. The rest of the backlog is
// a distraction to a worker, and an invitation to start what nobody assigned it.
func (e *Engine) CmdTasks(c registry.Caller, args []string, out io.Writer) (int, error) {
	home := e.taskHome(c)
	if err := e.SyncTasks(home); err != nil {
		return 1, err
	}
	ps := e.store.For(home)
	tasks, err := ps.AllTasks()
	if err != nil {
		return 1, err
	}
	visible, bounded, err := e.visibleTasks(c, tasks)
	if err != nil {
		return 1, err
	}

	if len(args) > 0 && args[0] != "list" {
		id := args[0]
		// A flag where an id belongs is a mistyped verb, not a task nobody has heard of: reading it as
		// an id answers "no such task", which sends the reader looking for the task.
		if strings.HasPrefix(id, "-") {
			fmt.Fprintf(out, "unknown argument %q\n%s\n", id, taskListUsage)
			return 2, nil
		}
		if bounded && !visible[id] {
			// Naming what it CAN read keeps the refusal actionable, and a worker that wandered
			// here was usually looking for its own package anyway.
			fmt.Fprintf(out, "%s is not part of your work. Run `sindri task` for the package you hold.\n", id)
			return 1, nil
		}
		t, err := e.TaskInfo(home, id)
		if err != nil {
			return 1, err
		}
		appr, comment := ps.GetApproval(t.ID)
		if comment != "" {
			appr += " — " + comment
		}
		// Type and labels are shown because a reviewer reads this: a `spec:<name>` label is what
		// tells it which spec the work must be verified against, and it lives nowhere else.
		fmt.Fprintf(out, "%s  [%s]  %s  priority=%s\napproval: %s\ntype:     %s\nlabels:   %s\nparent:   %s\nchildren: %s\n",
			t.ID, t.Status, t.Title, dash(t.Priority), dash(appr), dash(t.Type), dash(t.Labels),
			dash(t.ParentID), dash(strings.Join(childIDs(tasks, t.ID), ", ")))
		// Who holds it, for the roles that plan around people — the PR's author included, since the
		// name is most wanted once the work is submitted (-> api.AgentsByTask).
		if namesHolders(c.Role) {
			fmt.Fprintf(out, "agent:    %s\n", dash(e.holdersByTask(ps)[t.ID]))
		}
		fmt.Fprintf(out, "\n%s\n", dash(t.Description))
		// The same thread the TUI pane and `task info` show: an agent that just filed a finding
		// (-> the comment verb) has to be able to read it back here, or the verb is worse than none.
		fmt.Fprint(out, commentBlock(t.Comments))
		return 0, nil
	}
	if bounded && len(args) == 0 {
		return e.workerTaskView(c, tasks, out)
	}

	// The arguments after the `list` word — and a bare `task` carries none of them, which is how an
	// unbounded caller reaches here with an empty slice: only a BOUNDED one is answered above.
	rest := args
	if len(rest) > 0 {
		rest = rest[1:]
	}
	filter, ferr := parseTaskListFlags(rest)
	if ferr != nil {
		fmt.Fprintf(out, "%v\n%s\n", ferr, taskListUsage)
		return 2, nil
	}

	// Filtered, then given its ancestors back: the listing is indented by tree, so a subtask whose
	// parent the filter dropped would be re-rooted and read as belonging to nobody. The parents come
	// back as context, marked, so they are not mistaken for matches (-> api.WithAncestors).
	kept, context := api.WithAncestors(api.FilterTasks(filter, tasks), tasks)

	// Indent by depth so the parent/child structure is visible in the listing itself —
	// hierarchy is how work is organised here (an openspec change parents its tasks), and a
	// planner reads and repairs it from this view.
	prs, _ := ps.PRs()
	holders := map[string]string{}
	if namesHolders(c.Role) {
		holders = e.holdersByTask(ps)
	}
	shown, asContext := 0, 0
	for _, r := range task.ArrangeTasks(kept, prs) {
		if bounded && !visible[r.ID] {
			continue
		}
		note := ""
		if context[r.ID] {
			note, asContext = "  (context)", asContext+1
		} else {
			shown++
		}
		who := ""
		if namesHolders(c.Role) {
			who = fmt.Sprintf("%-12s ", dash(holders[r.ID]))
		}
		fmt.Fprintf(out, "%-12s %-8s %-9s %-3s %s%s%s%s\n",
			r.ID, r.Status, dash(r.Approval), dash(r.Priority), who, strings.Repeat("  ", r.Depth), r.Title, note)
	}
	if bounded && shown == 0 && asContext == 0 {
		fmt.Fprintln(out, "You hold no task. Run `sindri` to pick up your next one.")
		return 0, nil
	}
	// Counted over what the caller may READ, not the whole store: a worker's summary that quoted the
	// fleet's backlog would be describing a listing it is not being shown.
	fmt.Fprintln(out, api.TaskListSummary(filter, shown, readable(tasks, visible, bounded))+contextNote(asContext))
	return 0, nil
}

// contextNote accounts for the parent rows, so the count and the rows on screen agree.
func contextNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("; %d parent %s shown for context", n, map[bool]string{true: "task", false: "tasks"}[n == 1])
}

// readable is the tasks the caller may see, which for a planner, coauthor or reviewer is all of them.
func readable(tasks []store.Task, visible map[string]bool, bounded bool) []store.Task {
	if !bounded {
		return tasks
	}
	out := make([]store.Task, 0, len(visible))
	for _, t := range tasks {
		if visible[t.ID] {
			out = append(out, t)
		}
	}
	return out
}

// visibleTasks is what the caller may read, and whether that is BOUNDED rather than everything.
// A nil set with bounded=false means no filtering, so callers stay simple; a worker is bounded to
// its held task and every descendant — the unit the hub assigned it.
func (e *Engine) visibleTasks(c registry.Caller, tasks []store.Task) (map[string]bool, bool, error) {
	switch c.Role {
	case "planner", "coauthor", "reviewer":
		// A reviewer reads everything for the same reason a planner does: it judges work against
		// intent, and intent lives in the task, its neighbours and their comments. Reading grants no
		// authority — it still cannot claim, mutate, or act on anything but the PR it was handed.
		return nil, false, nil
	}
	st, err := e.store.For(c.Project).GetState(c.Agent)
	if err != nil {
		return nil, true, err
	}
	held := st.Container
	if held == "" {
		held = st.Task
	}
	visible := map[string]bool{}
	for _, r := range subtreeRows(tasks, held) {
		visible[r.ID] = true
	}
	return visible, true, nil
}

// workerTaskView answers bare `task` for a worker: what it holds. A standalone task prints in
// full, since there is nothing to choose between; a package prints as an overview small enough to
// re-read often, with the detail one request away.
func (e *Engine) workerTaskView(c registry.Caller, tasks []store.Task, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	held := st.Container // a package…
	if held == "" {
		held = st.Task // …else the single task
	}
	if held == "" {
		fmt.Fprintln(out, "You hold no task. Run `sindri` to pick up your next one.")
		return 0, nil
	}
	root, ok, err := ps.GetTask(held)
	if err != nil {
		return 1, err
	}
	if !ok {
		fmt.Fprintf(out, "You hold %s, but it is no longer in the backlog. Run `sindri` for your current directive.\n", held)
		return 0, nil
	}

	// GetTask reads the row; the thread lives in its own table and is fetched separately, the same
	// way TaskInfo attaches it. Bare `task` is where an agent looks first, so a comment addressed
	// to it has to arrive here — not only on the fuller `task <id>`.
	comments := e.deps.TaskComments(c.Project, root.ID)

	rows := subtreeRows(tasks, root.ID)
	if len(rows) <= 1 { // a standalone task: show it whole
		fmt.Fprintf(out, "Your task %s  [%s]  %s\n\n%s\n", root.ID, root.Status, root.Title, dash(root.Description))
		fmt.Fprint(out, commentBlock(comments))
		return 0, nil
	}
	fmt.Fprintf(out, "Your package %s: %s\n", root.ID, root.Title)
	if body := strings.TrimSpace(root.Description); body != "" {
		fmt.Fprintf(out, "\n%s\n", body)
	}
	// The package's own thread, not its subtasks' — a comment on the package is addressed to
	// whoever holds it, which is the reader. Each subtask carries its own to `task <id>`.
	fmt.Fprint(out, commentBlock(comments))
	fmt.Fprintf(out, "\n%d subtasks:\n", len(rows)-1)
	for _, r := range rows[1:] {
		marker := "  "
		if r.ID == st.Task {
			marker = "→ " // the subtask you are on now
		}
		fmt.Fprintf(out, "%s%-12s %-8s %s%s\n", marker, r.ID, r.Status, strings.Repeat("  ", r.Depth-1), r.Title)
	}
	fmt.Fprintln(out, "\n`sindri task <id>` shows any of them in full (description included).")
	return 0, nil
}

// subtreeRows is rootID and its descendants, depth-tagged in tree order — the shape an
// overview prints. Built from the cached task set, so it needs no extra read.
func subtreeRows(tasks []store.Task, rootID string) []task.TaskRow {
	byParent := map[string][]store.Task{}
	var root *store.Task
	for i, t := range tasks {
		if t.ID == rootID {
			root = &tasks[i]
		}
		if t.ParentID != "" {
			byParent[t.ParentID] = append(byParent[t.ParentID], t)
		}
	}
	if root == nil {
		return nil
	}
	var rows []task.TaskRow
	var walk func(t store.Task, depth int)
	walk = func(t store.Task, depth int) {
		rows = append(rows, task.TaskRow{Task: t, Depth: depth})
		for _, ch := range byParent[t.ID] {
			walk(ch, depth+1)
		}
	}
	walk(*root, 0)
	return rows
}

// namesHolders reports whether a role's task views name who holds each task: the roles that work
// around people. A worker sees only its own task, and a reviewer is told the author in DirReview.
func namesHolders(role string) bool { return role == "planner" || role == "coauthor" }

// holdersByTask names the agent behind each task by the rule both front-ends render, lifting the
// roster into the board's view type rather than restating it (-> api.AgentsByTask).
func (e *Engine) holdersByTask(ps *store.ProjectStore) map[string]string {
	roster, err := ps.Roster()
	if err != nil {
		return nil
	}
	views := make([]api.AgentView, 0, len(roster))
	for _, a := range roster {
		st, _ := ps.GetState(a.Name)
		views = append(views, api.AgentView{Name: a.Name, Task: st.Task, Feature: st.Container})
	}
	prs, _ := ps.PRs()
	return api.AgentsByTask(views, prs)
}
