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

// CmdTasks lets a planner read the backlog: `task list` prints every task indented by its
// place in the parent/child tree; `task <id>` prints that task's full detail, including its
// parent and children.
func (e *Engine) CmdTasks(c registry.Caller, args []string, out io.Writer) (int, error) {
	if err := e.SyncTasks(c.Project); err != nil {
		return 1, err
	}
	ps := e.store.For(c.Project)
	tasks, err := ps.AllTasks()
	if err != nil {
		return 1, err
	}
	if len(args) > 0 && args[0] != "list" {
		t, err := e.TaskInfo(c.Project, args[0])
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
	// Indent by depth so the parent/child structure is visible in the listing itself —
	// hierarchy is how work is organised here (an openspec change parents its tasks), and a
	// planner reads and repairs it from this view.
	prs, _ := ps.PRs()
	for _, r := range task.ArrangeTasks(tasks, prs) {
		fmt.Fprintf(out, "%-12s %-8s %-9s %-3s %s%s\n",
			r.ID, r.Status, dash(r.Approval), dash(r.Priority), strings.Repeat("  ", r.Depth), r.Title)
	}
	return 0, nil
}
