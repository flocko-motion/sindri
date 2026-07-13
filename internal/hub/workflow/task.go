// package: hub/workflow / task
// type:    logic (the act → report → idle loop + PR-as-merge-intent)
// job:     the worker verbs and task assignment. Tasks are a cached read model
//
//	synced from td (D15); `next` claims one and branches; the directive loop
//	decides the next action. All state is per-project — methods take a
//	project (repoTag) and work through store.For(project).
//
// limits:  git is entirely hub-side (the agent edits /workspace, the hub commits
//
//	and merges); writes to td go through the td adapter (D15).
package workflow

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/tasks/td"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// Tasks refreshes from td and returns all cached tasks for a project (for `task
// list`). A sync failure is surfaced, never swallowed.
func (e *Engine) Tasks(project string) ([]store.Task, error) {
	if err := e.SyncTasks(project); err != nil {
		return nil, err
	}
	// Repair any stale status (in_review with no PR, in_progress with no assignee)
	// against reality — a listing is a natural, infrequent point to do the sweep.
	_ = e.ReconcileTasks(project)
	return e.store.For(project).AllTasks()
}

// TaskInfo returns one task in a project, refreshed from the source of truth first.
// Only real td tasks (td-*) live in td's store; gh-* (GitHub issues) and os-* (spec)
// ids are synced into the hub's own cache with their description, so those are served
// from there — hitting td by a non-td id just errors and drops the description.
func (e *Engine) TaskInfo(project, id string) (store.Task, error) {
	if !strings.HasPrefix(id, "td-") {
		t, ok, err := e.store.For(project).GetTask(id)
		if err != nil {
			return store.Task{}, err
		}
		if !ok {
			return store.Task{}, fmt.Errorf("unknown task %q", id)
		}
		t.Comments = e.deps.TaskComments(project, id)
		return t, nil
	}
	// Repair this one task's status against reality before returning it (task info /
	// detail is a natural single-task check point).
	_ = e.ReconcileTask(project, id)
	root := e.deps.ProjectRoot(project)
	t, err := td.Get(root, id)
	if err != nil {
		return store.Task{}, err
	}
	st := ToStoreTask(t)
	if d, a, derr := td.Detail(root, id); derr == nil {
		st.Description, st.Acceptance = d, a
	}
	_ = e.store.For(project).UpsertTask(st)
	st.Comments = e.deps.TaskComments(project, id)
	return st, nil
}

// TaskSpec is the full editable shape of a task — the payload of both create and
// edit. Empty fields mean "unset" (create) or "leave unchanged" (edit).
type TaskSpec struct {
	Title       string
	Type        string
	Priority    string // a P-code (P0…P4)
	Parent      string // parent task id (a child of this task)
	Description string
	Labels      []string
}

// CreateTask creates a task via the td tool in a project and returns its id.
func (e *Engine) CreateTask(project string, s TaskSpec) (string, error) {
	if err := e.checkParent(project, s.Parent, ""); err != nil {
		return "", err
	}
	root := e.deps.ProjectRoot(project)
	out, err := td.Create(root, s.Title, td.CreateOpts{
		Type: s.Type, Priority: s.Priority, Body: s.Description, Labels: s.Labels, Parent: s.Parent,
	})
	if err != nil {
		return "", err
	}
	// td prints e.g. "CREATED td-1add0f" — return just the id.
	id := strings.TrimSpace(out)
	for _, f := range strings.Fields(out) {
		if strings.HasPrefix(f, "td-") {
			id = f
			break
		}
	}
	e.refreshCachedTask(project, id) // targeted: pull just the new task, not a full re-sync
	e.deps.Notify()
	return id, nil
}

// HealPlannerTasks releases any backlog task a planner is holding — an invalid
// assignment. Self-heals stale claims; runs once at hub boot, across all projects.
func (e *Engine) HealPlannerTasks() {
	agents, _ := e.store.AllAgents()
	for _, a := range agents {
		if a.Role != "planner" {
			continue
		}
		ps := e.store.For(a.Project)
		st, _ := ps.GetState(a.Name)
		if !strings.HasPrefix(st.Task, "td-") {
			continue
		}
		_ = td.SetStatus(e.deps.ProjectRoot(a.Project), st.Task, "open")
		_ = ps.SetState(store.AgentState{Agent: a.Name, Phase: "planning"})
		_ = ps.Log(a.Name, "unassign", st.Task+" (planners don't hold tasks)")
	}
}

// UnassignTask releases a task in a project back to the backlog and clears it from
// whatever agent held it. Refused if that agent is currently alive and working.
func (e *Engine) UnassignTask(project, id string) error {
	ps := e.store.For(project)
	roster, _ := ps.Roster()
	for _, a := range roster {
		st, _ := ps.GetState(a.Name)
		if st.Task != id {
			continue
		}
		if e.deps.AgentAlive(project, a.Name) {
			return fmt.Errorf("%s is alive and working on %s — stop or delete it first", a.Name, id)
		}
		_ = ps.SetState(store.AgentState{Agent: a.Name, Phase: "idle"})
		_ = ps.Log(a.Name, "unassign", id)
	}
	if strings.HasPrefix(id, "td-") {
		if err := td.SetStatus(e.deps.ProjectRoot(project), id, "open"); err != nil {
			return err
		}
	}
	_ = e.RefreshTask(project, id)
	e.deps.Notify()
	return nil
}

// ApproveTask clears the approval gate on a planner-proposed task (user-only),
// making it claimable, and tells any running planner in the project.
func (e *Engine) ApproveTask(project, id string) error {
	if err := e.store.For(project).SetApproval(id, "approved", ""); err != nil {
		return err
	}
	e.notifyPlanners(project, fmt.Sprintf("[user] task %s was approved — it's now in the backlog for a worker.", id))
	e.deps.Notify()
	return nil
}

// RejectTask rejects a planner-proposed task with a comment (user-only); it stays
// hidden from workers, and the comment is delivered to any running planner.
func (e *Engine) RejectTask(project, id, comment string) error {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		comment = "rejected"
	}
	if err := e.store.For(project).SetApproval(id, "rejected", comment); err != nil {
		return err
	}
	e.notifyPlanners(project, fmt.Sprintf("[user] task %s was rejected: %s", id, comment))
	e.deps.Notify()
	return nil
}

// notifyPlanners injects a message into every running planner's session in a project.
func (e *Engine) notifyPlanners(project, msg string) {
	roster, _ := e.store.For(project).Roster()
	for _, a := range roster {
		if a.Role == "planner" {
			name := a.Name
			go func() { _ = e.deps.InjectWhenReady(project, name, msg) }()
		}
	}
}

// CmdState lets a planner flip its own resting state between "planning" and "idle".
func (e *Engine) CmdState(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) != 1 || (args[0] != "planning" && args[0] != "idle") {
		return 1, fmt.Errorf("usage: state <planning|idle>")
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
	title := strings.TrimSpace(strings.Join(args, " "))
	if title == "" {
		return 1, fmt.Errorf("usage: create-task <title...>")
	}
	id, err := e.CreateTask(c.Project, TaskSpec{Title: title, Type: "task"})
	if err != nil {
		return 1, err
	}
	if err := e.store.For(c.Project).SetApproval(id, "pending", ""); err != nil {
		return 1, err
	}
	e.deps.Notify()
	fmt.Fprintln(out, ReplyTaskProposed(id, title))
	return 0, nil
}

// CmdTasks lets a planner read the backlog: `task list` lists every task; `task
// <id>` prints that task's full detail.
func (e *Engine) CmdTasks(c registry.Caller, args []string, out io.Writer) (int, error) {
	if err := e.SyncTasks(c.Project); err != nil {
		return 1, err
	}
	ps := e.store.For(c.Project)
	if len(args) > 0 && args[0] != "list" {
		t, err := e.TaskInfo(c.Project, args[0])
		if err != nil {
			return 1, err
		}
		appr, comment := ps.GetApproval(t.ID)
		if comment != "" {
			appr += " — " + comment
		}
		fmt.Fprintf(out, "%s  [%s]  %s  priority=%s\napproval: %s\n\n%s\n",
			t.ID, t.Status, t.Title, dash(t.Priority), dash(appr), dash(t.Description))
		return 0, nil
	}
	tasks, err := ps.AllTasks()
	if err != nil {
		return 1, err
	}
	for _, t := range tasks {
		fmt.Fprintf(out, "%-12s %-8s %-9s %-3s %s\n", t.ID, t.Status, dash(t.Approval), dash(t.Priority), t.Title)
	}
	return 0, nil
}

// dash renders "-" for an empty string (agent-facing output helper).
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// EditTask applies a spec to an existing task in a project.
func (e *Engine) EditTask(project, id string, s TaskSpec) error {
	if err := e.checkParent(project, s.Parent, id); err != nil {
		return err
	}
	if strings.HasPrefix(id, "td-") {
		if err := td.Update(e.deps.ProjectRoot(project), id, td.UpdateOpts{
			Title: s.Title, Type: s.Type, Priority: s.Priority, Body: s.Description, Labels: s.Labels, Parent: s.Parent,
		}); err != nil {
			return err
		}
	} else if s.Priority != "" {
		if err := e.store.For(project).SetPriorityOverride(id, s.Priority); err != nil {
			return err
		}
	}
	e.refreshCachedTask(project, id) // targeted refresh of the edited task
	e.deps.Notify()
	return nil
}

// workPollInterval re-checks for work while a directive is parked.
const workPollInterval = 3 * time.Second

// prRejected reports whether an agent has a rejected PR in its project (the signal to
// revise, not wait) and returns the reviewer's feedback, so the worker can be handed
// the comments directly rather than having to go find them.
func (e *Engine) prRejected(project, agent string) (feedback string, rejected bool, err error) {
	prs, err := e.store.For(project).PRs()
	if err != nil {
		return "", false, fmt.Errorf("load PRs for %s: %w", agent, err)
	}
	for _, p := range prs {
		if p.Agent == agent && p.Status == "rejected" {
			return p.Feedback, true, nil
		}
	}
	return "", false, nil
}

// workDirective is what a working agent is told: if its PR was rejected, the
// reviewer's feedback is PUSHED (every time it asks — it never has to go hunting for
// why the PR bounced); otherwise the plain "work on the task" directive.
func (e *Engine) workDirective(project, name, task string) (string, error) {
	feedback, rejected, err := e.prRejected(project, name)
	if err != nil {
		return "", err
	}
	if rejected {
		return DirRejected(task, feedback), nil
	}
	return DirWorking(task), nil
}

// AgentDirective is the single next action the hub wants this agent to take — the
// no-arg `sindri` answer. The hub decides; the agent obeys. When there's nothing to
// do it BLOCKS until there is. ctx cancels the wait when the pod dies.
func (e *Engine) AgentDirective(ctx context.Context, project, name string) (string, error) {
	ps := e.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("unknown agent %q", name)
	}
	if a.Role == "coauthor" {
		return DirCoauthor, nil
	}
	if a.Role == "reviewer" {
		return e.waitForWork(ctx, func() (string, bool, error) {
			prs, err := ps.PRs()
			if err != nil {
				return "", false, err
			}
			for _, pr := range prs {
				if pr.Status == "open" {
					return DirReview(pr.ID, pr.Task, e.deps.ArchitectureDoc(project)), true, nil
				}
			}
			return "", false, nil
		})
	}
	st, _ := ps.GetState(name)
	if a.Role == "planner" {
		if st.Phase == "submitted" {
			return DirSubmitted, nil
		}
		return DirPlanner, nil
	}
	// A worker holding a container is in the collaborative loop.
	if st.Container != "" {
		if t, ok, _ := ps.GetTask(st.Container); ok && t.Status != "closed" && t.Status != "approved" && t.Status != "merged" {
			switch st.Phase {
			case "submitted":
				feedback, rejected, err := e.prRejected(project, name)
				if err != nil {
					return "", err
				}
				if rejected {
					_ = ps.SetState(store.AgentState{Agent: name, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "working"})
					return DirRejected(st.Task, feedback), nil
				}
				return DirSubmitted, nil
			case "working":
				return e.workDirective(project, name, st.Task)
			default:
				if next, ok := e.advanceContainer(project, name, st.Container); ok {
					return DirWorking(next.ID), nil
				}
				return DirContainerWait(st.Container), nil
			}
		}
		_ = ps.SetState(store.AgentState{Agent: name, Phase: "idle"})
		return e.waitForWork(ctx, func() (string, bool, error) { return e.claimNext(project, name) })
	}
	switch st.Phase {
	case "working":
		return e.workDirective(project, name, st.Task)
	case "submitted":
		feedback, rejected, err := e.prRejected(project, name)
		if err != nil {
			return "", err
		}
		if rejected {
			_ = ps.SetState(store.AgentState{Agent: name, Task: st.Task, Branch: st.Branch, Phase: "working"})
			return DirRejected(st.Task, feedback), nil
		}
		return DirSubmitted, nil
	default: // idle — claim the next task, blocking until one exists
		return e.waitForWork(ctx, func() (string, bool, error) { return e.claimNext(project, name) })
	}
}

// waitForWork blocks until check reports work is ready (returning its directive) or
// ctx is cancelled. Re-checks on every hub change and on a short timer.
func (e *Engine) waitForWork(ctx context.Context, check func() (string, bool, error)) (string, error) {
	ch, unsub := e.deps.Subscribe()
	defer unsub()
	for {
		d, ready, err := check()
		if err != nil {
			return "", err
		}
		if ready {
			return d, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ch: // a hub mutation — re-check
		case <-time.After(workPollInterval): // re-sync td and re-check
		}
	}
}

// SyncTasks refreshes a project's cached task set from its sources (td + openspec +
// the TTL-throttled GitHub scan). ForceSyncTasks bypasses the GitHub TTL for an
// explicit [r]efresh.
func (e *Engine) SyncTasks(project string) error { return e.syncTasks(project, false) }

// ForceSyncTasks is SyncTasks with the GitHub scan forced past its TTL ([r]efresh).
func (e *Engine) ForceSyncTasks(project string) error { return e.syncTasks(project, true) }

func (e *Engine) syncTasks(project string, force bool) error {
	root := e.deps.ProjectRoot(project)
	ps := e.store.For(project)
	var rows []store.Task

	// Every task source, treated identically — the hub never branches on which one it
	// is. Each Source self-gates (Enabled), normalizes to task.Task with its own id
	// scheme, and (for a network source) throttles + degrades internally; force asks
	// for fresh data. td errors fail the sync (it's the primary store); a network
	// source degrades to its last good list rather than erroring.
	for _, src := range taskSources() {
		if !src.Enabled(root) {
			continue
		}
		ts, err := src.Tasks(root, force)
		if err != nil {
			return err
		}
		for _, t := range ts {
			rows = append(rows, ToStoreTask(t))
		}
	}

	if ov, err := ps.PriorityOverrides(); err == nil {
		for i := range rows {
			if p, ok := ov[rows[i].ID]; ok {
				rows[i].Priority = p
			}
		}
	}
	return ps.ReplaceTasks(rows)
}

// SetPriority assigns a task's priority (a P-code) in a project.
func (e *Engine) SetPriority(project, id, priority string) error {
	if strings.HasPrefix(id, "td-") {
		if err := td.SetPriority(e.deps.ProjectRoot(project), id, priority); err != nil {
			return err
		}
	} else {
		if err := e.store.For(project).SetPriorityOverride(id, priority); err != nil {
			return err
		}
	}
	e.refreshCachedTask(project, id) // targeted refresh of the reprioritized task
	e.deps.Notify()
	return nil
}

// checkParent validates a requested parent id within a project.
func (e *Engine) checkParent(project, parent, self string) error {
	if parent == "" {
		return nil
	}
	if parent == self {
		return fmt.Errorf("a task can't be its own parent")
	}
	tasks, err := e.store.For(project).AllTasks()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.ID == parent {
			return nil
		}
	}
	return fmt.Errorf("unknown parent %q", parent)
}

// ToStoreTask maps a source-normalized domain task onto the hub's cached store row.
// Exported because the hub's targeted single-task refresh reuses the same mapping.
func ToStoreTask(t task.Task) store.Task {
	return store.Task{
		ID: t.ID, Title: t.Title, Status: t.Status, Priority: t.Priority,
		Type: t.Type, Labels: strings.Join(t.Labels, ","), ParentID: t.ParentID,
		Description: t.Description,
	}
}

// CmdNext claims the highest-priority open task for a worker and branches for it.
func (e *Engine) CmdNext(c registry.Caller, _ []string, out io.Writer) (int, error) {
	d, claimed, err := e.claimNext(c.Project, c.Agent)
	if err != nil {
		return 1, err
	}
	if !claimed {
		fmt.Fprintln(out, DirNoTasks)
		return 0, nil
	}
	fmt.Fprintln(out, d)
	return 0, nil
}

// claimNext claims the highest-priority open LEAF task (or a marked container) for a
// worker in a project. Returns (directive, true) on a claim, ("", false) when idle.
func (e *Engine) claimNext(project, agent string) (string, bool, error) {
	_ = e.SyncTasks(project) // best-effort refresh; cached set on failure
	if d, ok, err := e.claimContainer(project, agent); ok || err != nil {
		return d, ok, err
	}
	return e.claimLeaf(project, agent)
}

// claimLeaf claims the highest-priority open leaf for a worker, branching on it.
func (e *Engine) claimLeaf(project, worker string) (string, bool, error) {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	open, err := ps.OpenLeaves()
	if err != nil {
		return "", false, err
	}
	if len(open) == 0 {
		return "", false, nil
	}
	t := open[0]
	base, err := e.baseBranch(root)
	if err != nil {
		return "", false, err
	}
	a, ok, err := ps.GetAgent(worker)
	if err != nil || !ok {
		return "", false, fmt.Errorf("agent %s missing: %v", worker, err)
	}
	wt := filepath.Join(root, a.Workspace)
	branch := t.ID
	// Only td owns a task's status. A gh-* issue's "in_progress" lives in agent_state
	// (which OpenLeaves honors) — GitHub isn't told a worker started; the issue is
	// touched only on merge (close+comment). Calling td for a gh-/os- id would error.
	if strings.HasPrefix(t.ID, "td-") {
		if err := td.SetStatus(root, t.ID, "in_progress"); err != nil {
			return "", false, err
		}
		_ = e.RefreshTask(project, t.ID)
	}
	// Lay the new branch on a CLEAN base. A prior task's leftover WIP — e.g. a task
	// cancelled out from under this agent while it kept editing — would otherwise
	// block `checkout -B` or bleed into the new branch. The hub owns git, so it
	// resets here, at claim time (not at cancel time, since the agent may work on
	// after the cancel push) — the agent never cleans up its own worktree.
	if err := git.CheckoutDetachedClean(wt, base); err != nil {
		return "", false, err
	}
	if err := git.CreateBranch(wt, branch, base); err != nil {
		return "", false, err
	}
	if err := ps.SetState(store.AgentState{Agent: worker, Task: t.ID, Branch: branch, Phase: "working"}); err != nil {
		return "", false, err
	}
	_ = ps.Log(worker, "claim", t.ID+" "+t.Title)
	e.deps.Notify()
	return DirClaimed(t.ID, t.Title, branch, e.deps.ArchitectureDoc(project)), true, nil
}

// collabLabel marks a parent task for collaborative assignment.
const collabLabel = "collab"

// claimContainer assigns the highest-priority marked, unheld container in a project
// to the agent, starting it on the container's first open child.
func (e *Engine) claimContainer(project, worker string) (string, bool, error) {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	containers, err := ps.MarkedContainers(collabLabel)
	if err != nil || len(containers) == 0 {
		return "", false, err
	}
	c := containers[0]
	children, err := ps.OpenChildren(c.ID)
	if err != nil {
		return "", false, err
	}
	if len(children) == 0 {
		return "", false, nil // marked but nothing open to work
	}
	base, err := e.baseBranch(root)
	if err != nil {
		return "", false, err
	}
	a, ok, err := ps.GetAgent(worker)
	if err != nil || !ok {
		return "", false, fmt.Errorf("agent %s missing: %v", worker, err)
	}
	wt := filepath.Join(root, a.Workspace)
	if err := git.EnsureBranch(wt, c.ID, base); err != nil {
		return "", false, err
	}
	child := children[0]
	if err := td.SetStatus(root, child.ID, "in_progress"); err != nil {
		return "", false, err
	}
	_ = e.RefreshTask(project, child.ID)
	if err := ps.SetState(store.AgentState{Agent: worker, Container: c.ID, Branch: c.ID, Task: child.ID, Phase: "working"}); err != nil {
		return "", false, err
	}
	_ = ps.Log(worker, "claim-container", c.ID+" "+c.Title)
	e.deps.Notify()
	return DirContainerClaimed(c.ID, c.Title, child.ID, child.Title), true, nil
}

// CmdCheckpoint commits the current subtask to the container branch, closes that
// child, and advances to the next — staying working, never blocking for review.
func (e *Engine) CmdCheckpoint(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	root := e.deps.ProjectRoot(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Container == "" || st.Phase != "working" || st.Task == "" {
		fmt.Fprintln(out, ReplyNothingToCheckpoint)
		return 1, nil
	}
	a, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, a.Workspace)
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		msg = "work on " + st.Task
	}
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	if err := td.SetStatus(root, st.Task, "closed"); err != nil {
		return 1, err
	}
	_ = e.RefreshTask(c.Project, st.Task)
	_ = ps.Log(c.Agent, "checkpoint", st.Task)
	done := st.Task
	if next, ok := e.advanceContainer(c.Project, c.Agent, st.Container); ok {
		fmt.Fprintln(out, ReplyCheckpointed(done, next.ID, next.Title))
		return 0, nil
	}
	_ = ps.SetState(store.AgentState{Agent: c.Agent, Container: st.Container, Branch: st.Container, Phase: "idle"})
	e.deps.Notify()
	fmt.Fprintln(out, ReplyCheckpointedLast(done, st.Container))
	return 0, nil
}

// advanceContainer moves a held container's agent onto its next open child in a
// project, returning (child, true) when one was assigned or (zero, false) if none.
func (e *Engine) advanceContainer(project, agent, container string) (store.Task, bool) {
	ps := e.store.For(project)
	children, err := ps.OpenChildren(container)
	if err != nil || len(children) == 0 {
		return store.Task{}, false
	}
	child := children[0]
	if err := td.SetStatus(e.deps.ProjectRoot(project), child.ID, "in_progress"); err != nil {
		return store.Task{}, false
	}
	_ = e.RefreshTask(project, child.ID)
	_ = ps.SetState(store.AgentState{Agent: agent, Container: container, Branch: container, Task: child.ID, Phase: "working"})
	e.deps.Notify()
	return child, true
}
