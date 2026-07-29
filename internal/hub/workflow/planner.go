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

// AssignPlan hands a planner one thing to plan, as a phased brief (-> MsgPlanAssignment). Refused
// while it has a PR open: it drafts on ONE standing branch, so a second plan would pile
// unreviewed work onto specs awaiting a verdict.
func (e *Engine) AssignPlan(project, agent, goal string) error {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return fmt.Errorf("say what to plan: a goal, question or feature to work out")
	}
	ps := e.store.For(project)
	a, ok, err := ps.GetAgent(agent)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", agent)
	}
	if a.Role != "planner" {
		return fmt.Errorf("%s is a %s — planning is a planner's job (`sindri agent new --role planner`)", agent, a.Role)
	}
	if pr, found, perr := e.openPlannerPR(ps, agent); perr != nil {
		return perr
	} else if found {
		return fmt.Errorf("%s still has %s open — merge it (`sindri pr merge %s`) or scrap it "+
			"(`sindri pr scrap %s --yes`) before assigning a new plan, or the new work would be "+
			"drafted on top of specs nobody has ruled on yet", agent, pr.ID, pr.ID, pr.ID)
	}

	// Interrupt first: the directive has to land on an idle prompt, or it queues behind whatever
	// the agent is already doing and arrives after the work it was meant to redirect.
	if e.deps.AgentAlive(project, agent) {
		_ = e.deps.Interrupt(project, agent)
	}
	if err := e.deps.InjectWhenReady(project, agent, MsgPlanAssignment(goal, e.deps.ArchitectureDoc(project), e.planReading(project))); err != nil {
		return err
	}
	st, _ := ps.GetState(agent)
	st.Agent, st.Phase = agent, "planning"
	_ = ps.SetState(st)
	_ = ps.Log(agent, "plan", goal)
	e.deps.Notify()
	return nil
}

// planReading is the project's configured reading list, as one /workspace-rooted phrase for the
// assignment. Empty when unconfigured, and the phase simply drops out.
func (e *Engine) planReading(project string) string {
	cfg, err := e.deps.ProjectConfig(project)
	if err != nil || len(cfg.Reading) == 0 {
		return ""
	}
	paths := make([]string, 0, len(cfg.Reading))
	for _, p := range cfg.Reading {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, "/workspace/"+strings.TrimPrefix(p, "/"))
		}
	}
	return strings.Join(paths, ", ")
}

// openPlannerPR is the planner's own PR still awaiting a verdict, if any.
func (e *Engine) openPlannerPR(ps *store.ProjectStore, agent string) (store.PR, bool, error) {
	prs, err := ps.PRs()
	if err != nil {
		return store.PR{}, false, err
	}
	for _, pr := range prs {
		if pr.Agent == agent && pr.Status != "merged" && pr.Status != "scrapped" {
			return pr, true, nil
		}
	}
	return store.PR{}, false, nil
}

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

// parseTaskFlags splits create-task's flags from the words forming the title, accepting both
// `--flag value` and `--flag=value`. An unknown flag is an error: silently ignoring one creates
// the task without the parent or body that was asked for. Priority is absent on purpose — it is
// what releases work to a worker (-> store.OpenLeaves), so it stays the user's.
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

// CmdEditTask repairs a planner's own proposal — retitle, body, or a parent, which is how flat
// proposals become a tree. PENDING only: approval is the user taking the task as it stands, so
// what a worker picks up is what they read.
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

// CmdTasks is the read surface over the backlog, scoped to the caller's job: a planner or
// coauthor shapes all of it, a worker sees only the package it holds. The rest of the backlog is
// a distraction to a worker, and an invitation to start what nobody assigned it.
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

// visibleTasks is what the caller may read, and whether that is BOUNDED rather than everything.
// A nil set with bounded=false means no filtering, so callers stay simple; a worker is bounded to
// its held task and every descendant — the unit the hub assigned it.
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
