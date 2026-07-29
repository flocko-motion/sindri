// package: hub/workflow / planner
// type:    logic (the planner's verb surface)
// job:     the verbs a planner drives the backlog with — read it as a tree (task list,
//          task <id>), propose work into it (create-task, parented so a proposal joins an
//          existing change or task), and set its own resting state.
// limits:  command surface only; task creation itself is task.go's (CreateTask) and
//          persistence the store's.
package workflow

import (
	"fmt"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// CmdState lets a planner flip its own resting state between "planning" and "idle".
func (e *Engine) CmdState(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) != 1 || (args[0] != "planning" && args[0] != "idle") {
		fmt.Fprintln(out, "usage: state <planning|idle>")
		return 2, nil
	}
	ps := e.store.For(c.Project)
	st, _ := ps.GetState(c.Agent)
	st.Agent, st.Phase = c.Agent, args[0]
	if err := ps.SetState(st); err != nil {
		return 1, err
	}
	e.deps.Notify()
	fmt.Fprintf(out, "state: %s\n", args[0])
	return 0, nil
}

// CmdCreateTask lets a planner propose a task, flagged pending the user's approval.
func (e *Engine) CmdCreateTask(c registry.Caller, args []string, out io.Writer) (int, error) {
	spec, words, perr := parseTaskFlags(args)
	if perr != nil {
		fmt.Fprintf(out, "%v\n%s\n", perr, createTaskUsage)
		return 2, nil
	}
	spec.Title = strings.TrimSpace(strings.Join(words, " "))
	if spec.Title == "" {
		fmt.Fprintln(out, createTaskUsage)
		return 2, nil
	}
	if spec.Type == "" {
		spec.Type = "task"
	}
	id, err := e.CreateTask(c.Project, spec)
	if err != nil {
		// A rejected parent is the caller's to fix, so it goes to them rather than to the
		// operator's log as an internal fault.
		fmt.Fprintf(out, "could not create the task: %v\n", err)
		return 1, nil
	}
	if err := e.store.For(c.Project).SetApproval(id, "pending", ""); err != nil {
		return 1, err
	}
	e.deps.Notify()
	fmt.Fprintln(out, ReplyTaskProposed(id, spec.Title))
	return 0, nil
}

// createTaskUsage is the one description of create-task's surface, shown for a bad flag, a
// missing title, and (via CreateTaskHelp) `create-task --help`.
const createTaskUsage = "usage: create-task [--parent <id>] [--type <task|feature|bug|epic>] [--body <text>] [--labels a,b] <title...>\n" +
	"  --parent  hang the task under an existing task or openspec change (os-*), so it joins that tree\n" +
	"  --body    the task's description — what a worker needs in order to start\n" +
	"The user sets the priority when they approve: a task without one is never handed to a worker,\n" +
	"so approval and prioritisation are the two human decisions that release work."

// CreateTaskHelp is what the command registry advertises for create-task, so the verb list
// and the verb's own usage describe one surface.
const CreateTaskHelp = "propose a new task, needing the user's approval. " + createTaskUsage

// parseTaskFlags reads create-task's flags, returning the spec they build and the leftover
// words that form the title. Both `--flag value` and `--flag=value` are accepted.
//
// An unrecognised flag is an error the caller sees: a silently ignored option looks like it
// took effect, and the task is then created without the parent or body that was asked for.
//
// Priority is deliberately absent. store.OpenLeaves hands out only tasks that are approved
// AND carry a priority, so the priority a human sets at approval time is the signal that
// releases work to a worker — it belongs to the user, alongside the approval itself.
func parseTaskFlags(args []string) (TaskSpec, []string, error) {
	var s TaskSpec
	var words []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			words = append(words, a)
			continue
		}
		name, inline, hasInline := strings.Cut(a, "=")
		val := inline
		if !hasInline {
			if i+1 >= len(args) {
				return s, nil, fmt.Errorf("%s needs a value", name)
			}
			i++
			val = args[i]
		}
		switch name {
		case "--parent", "-P":
			s.Parent = val
		case "--type", "-t":
			s.Type = val
		case "--body", "--description", "-d":
			s.Description = val
		case "--labels", "-l":
			s.Labels = strings.Split(val, ",")
		default:
			return s, nil, fmt.Errorf("unknown flag %q", name)
		}
	}
	return s, words, nil
}

// editTaskUsage is the one description of edit-task's surface.
const editTaskUsage = "usage: edit-task <id> [--parent <id>] [--type <task|feature|bug|epic>] [--body <text>] [--labels a,b] [<new title...>]\n" +
	"  --parent  hang this task under another task or openspec change — how a set of flat\n" +
	"            proposals becomes a tree: propose the parent, then point each child at it\n" +
	"  Only a task still awaiting the user's approval can be edited. Omitted fields are left as they are."

// EditTaskHelp is what the command registry advertises for edit-task.
const EditTaskHelp = "revise a task you proposed, while it still awaits approval. " + editTaskUsage

// CmdEditTask lets a planner repair its own proposal: retitle it, give it a body, or hang it
// under a parent — the move that turns a set of flat proposals into a tree.
//
// It applies only while the task is PENDING. Approval is the user's decision to take the
// task as it stands, so from that moment the task is theirs: what a worker picks up is what
// the user read and released.
func (e *Engine) CmdEditTask(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(out, editTaskUsage)
		return 2, nil
	}
	id := args[0]
	spec, words, perr := parseTaskFlags(args[1:])
	if perr != nil {
		fmt.Fprintf(out, "%v\n%s\n", perr, editTaskUsage)
		return 2, nil
	}
	spec.Title = strings.TrimSpace(strings.Join(words, " "))
	if spec.Title == "" && spec.Parent == "" && spec.Type == "" && spec.Description == "" && len(spec.Labels) == 0 {
		fmt.Fprintf(out, "nothing to change on %s\n%s\n", id, editTaskUsage)
		return 2, nil
	}
	ps := e.store.For(c.Project)
	appr, _ := ps.GetApproval(id)
	if appr != "pending" {
		fmt.Fprintf(out, "%s can't be edited: only a task still awaiting the user's approval can be (this one is %s). "+
			"Propose a new task instead, or ask the user in the meeting room.\n", id, dash(appr))
		return 1, nil
	}
	if err := e.EditTask(c.Project, id, spec); err != nil {
		// A rejected parent (unknown id, or a cycle) is the caller's to fix.
		fmt.Fprintf(out, "could not edit %s: %v\n", id, err)
		return 1, nil
	}
	_ = e.RefreshTask(c.Project, id)
	e.deps.Notify()
	fmt.Fprintf(out, "%s updated — still awaiting the user's approval.\n", id)
	return 0, nil
}

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

// TaskHelp is what the registry advertises for `task`.
const TaskHelp = "read your work: `task` (your own task or package; a planner or coauthor: the whole backlog), " +
	"`task <id>` (one task in full — description, parent, children), `task list` (every task you can see, indented by tree)"

// CmdTasks is the read surface over the backlog. What it shows depends on the caller's job.
//
// A planner or coauthor shapes the whole backlog and sees all of it. A WORKER sees only the
// package it holds — that task and its descendants. Its job is to finish one piece of work
// well, and the rest of the backlog is at best a distraction and at worst an invitation to
// start something nobody assigned it. Bounding the view keeps the agent's attention where the
// hub put it, and keeps a long backlog out of a context window that has better uses.
func (e *Engine) CmdTasks(c registry.Caller, args []string, out io.Writer) (int, error) {
	if err := e.SyncTasks(c.Project); err != nil {
		return 1, err
	}
	ps := e.store.For(c.Project)
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
		if bounded && !visible[id] {
			// Naming what it CAN read keeps the refusal actionable, and a worker that wandered
			// here was usually looking for its own package anyway.
			fmt.Fprintf(out, "%s is not part of your work. Run `sindri task` for the package you hold.\n", id)
			return 1, nil
		}
		t, err := e.TaskInfo(c.Project, id)
		if err != nil {
			return 1, err
		}
		appr, comment := ps.GetApproval(t.ID)
		if comment != "" {
			appr += " — " + comment
		}
		fmt.Fprintf(out, "%s  [%s]  %s  priority=%s\napproval: %s\nparent:   %s\nchildren: %s\n\n%s\n",
			t.ID, t.Status, t.Title, dash(t.Priority), dash(appr),
			dash(t.ParentID), dash(strings.Join(childIDs(tasks, t.ID), ", ")), dash(t.Description))
		return 0, nil
	}
	if bounded && len(args) == 0 {
		return e.workerTaskView(c, tasks, out)
	}

	// Indent by depth so the parent/child structure is visible in the listing itself —
	// hierarchy is how work is organised here (an openspec change parents its tasks), and a
	// planner reads and repairs it from this view.
	prs, _ := ps.PRs()
	shown := 0
	for _, r := range task.ArrangeTasks(tasks, prs) {
		if bounded && !visible[r.ID] {
			continue
		}
		shown++
		fmt.Fprintf(out, "%-12s %-8s %-9s %-3s %s%s\n",
			r.ID, r.Status, dash(r.Approval), dash(r.Priority), strings.Repeat("  ", r.Depth), r.Title)
	}
	if bounded && shown == 0 {
		fmt.Fprintln(out, "You hold no task. Run `sindri` to pick up your next one.")
	}
	return 0, nil
}

// visibleTasks is the set of tasks the caller may read, and whether that set is BOUNDED (a
// subset) rather than the whole backlog.
//
// Planners and coauthors work across the backlog, so they are unbounded and the set is nil — a
// nil map with bounded=false means "no filtering", which keeps the caller's checks trivial. A
// worker is bounded to the package it holds: the task recorded in its state, plus every
// descendant, which is exactly the unit the hub assigned it.
func (e *Engine) visibleTasks(c registry.Caller, tasks []store.Task) (map[string]bool, bool, error) {
	switch c.Role {
	case "planner", "coauthor":
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

// workerTaskView answers bare `task` for a worker: what it currently holds.
//
// A standalone task prints in full — there is nothing to choose between, so making the worker
// ask again would be a wasted step. A package prints as an overview instead: the parent, then
// every descendant with its status and a marker on the current subtask, ending with the one
// command that opens any of them. The overview stays small enough to re-read often, and the
// detail is a request away rather than a wall of text the worker didn't ask for.
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

	rows := subtreeRows(tasks, root.ID)
	if len(rows) <= 1 { // a standalone task: show it whole
		fmt.Fprintf(out, "Your task %s  [%s]  %s\n\n%s\n", root.ID, root.Status, root.Title, dash(root.Description))
		return 0, nil
	}
	fmt.Fprintf(out, "Your package %s: %s\n", root.ID, root.Title)
	if body := strings.TrimSpace(root.Description); body != "" {
		fmt.Fprintf(out, "\n%s\n", body)
	}
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
