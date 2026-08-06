// package: hub/workflow / task
// type:    logic (the act → report → idle loop + PR-as-merge-intent)
// job:     the worker verbs and task assignment. Tasks are a cached read model
// synced from td (D15); `next` claims one and branches; the directive loop
// decides the next action. All state is per-project — methods take a
// project (repoTag) and work through store.For(project).
// limits:  git is entirely hub-side (the agent edits /workspace, the hub commits
// and merges); writes to td go through the td adapter (D15).
package workflow

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
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

// TaskInfo returns one task, refreshed from its source of truth. Only td-* live in td's store;
// gh-* and os-* are served from the hub's cache, since asking td by a non-td id only errors.
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
	ps := e.store.For(project)
	owned, ok, err := ps.OwnedTask(id)
	if err != nil {
		return store.Task{}, err
	}
	if !ok {
		return store.Task{}, fmt.Errorf("no such task %q", id)
	}
	_ = ps.UpsertTask(store.Task{
		ID: owned.ID, Title: owned.Title, Status: owned.Status, Priority: owned.Priority,
		Type: owned.Type, Labels: owned.Labels, ParentID: ps.ParentOf(id),
		Description: owned.Description, UpdatedAt: owned.UpdatedAt,
	})
	// Read the row back rather than returning what was just written: the approval gate lives in its
	// own table and reaches a task only through that join, so a hand-built row reports none.
	st, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		return store.Task{}, err
	}
	st.Comments = e.deps.TaskComments(project, id)
	return st, nil
}

// TaskSpec is the full editable shape of a task — the payload of both create and
// edit. It crosses the wire, so it is internal/api.TaskSpec under the name every
// existing caller here already uses.
type TaskSpec = api.TaskSpec

// CreateTask creates a task via the td tool in a project and returns its id.
func (e *Engine) CreateTask(project string, s TaskSpec) (string, error) {
	if err := e.checkParent(project, s.Parent, ""); err != nil {
		return "", err
	}
	id, err := NewOwnedID()
	if err != nil {
		return "", err
	}
	typ := s.Type
	if typ == "" {
		typ = "task"
	}
	ps := e.store.For(project)
	if err := ps.PutOwnedTask(store.OwnedTask{
		ID: id, Title: s.Title, Status: "open", Priority: s.Priority, Type: typ,
		Labels: strings.Join(s.Labels, ","), Description: s.Description,
	}); err != nil {
		return "", err
	}
	if err := ps.SetParent(id, s.Parent); err != nil {
		return "", err
	}
	e.refreshCachedTask(project, id) // targeted: pull just the new task, not a full re-sync
	e.deps.Notify()
	e.nudgeIdleWorkers(project, id, s.Priority)
	return id, nil
}

// nudgeIdleWorkers tells idle workers rated work exists. Notify only wakes one already blocked in
// waitForWork; an agent that asked, got nothing and stopped asking would sit beside a claimable task
// forever. Unrated tasks are skipped — a worker can't claim one, so the nudge would be noise.
func (e *Engine) nudgeIdleWorkers(project, id, priority string) {
	if priority == "" {
		return
	}
	ps := e.store.For(project)
	agents, err := ps.Roster()
	if err != nil {
		return
	}
	for _, a := range agents {
		if a.Role != "worker" {
			continue // only workers claim backlog tasks
		}
		st, _ := ps.GetState(a.Name)
		if st.Task != "" || (st.Phase != "" && st.Phase != "idle") {
			continue // holding work, or mid-flow — leave it alone
		}
		if !e.deps.AgentAlive(project, a.Name) {
			continue // nothing to inject into
		}
		_ = e.deps.InjectWhenReady(project, a.Name, MsgWorkAvailable(id))
		_ = ps.Log(a.Name, "nudge", "work available: "+id)
	}
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
		_ = ps.SetOwnedStatus(st.Task, "open")
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
	if ps.OwnsTask(id) {
		if err := ps.SetOwnedStatus(id, "open"); err != nil {
			return err
		}
	}
	_ = e.RefreshTask(project, id)
	e.deps.Notify()
	return nil
}

// The approval gate (approve/reject) lives in approval.go; the planner's verb surface in planner.go.

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
	ps := e.store.For(project)
	// Parentage first, and for any task: the hierarchy is sindri's own, so re-parenting an openspec
	// change or a GitHub issue is as ordinary as re-parenting one of its own.
	if s.Parent != "" {
		if err := ps.SetParent(id, s.Parent); err != nil {
			return err
		}
	}
	if owned, ok, oerr := ps.OwnedTask(id); oerr != nil {
		return oerr
	} else if ok {
		// Only what the spec carries changes; an empty field leaves the stored one as it is.
		applySpec(&owned, s)
		if err := ps.PutOwnedTask(owned); err != nil {
			return err
		}
	} else if s.Priority != "" {
		if err := ps.SetPriorityOverride(id, s.Priority); err != nil {
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
// container is the feature it holds, if any: it decides which verb the directive names, and MUST
// match what the command registry shows that caller — a directive is an instruction to obey, so one
// naming a hidden verb leaves the agent to improvise the workflow.
func (e *Engine) workDirective(project, name, task, container string) (string, error) {
	feedback, rejected, err := e.prRejected(project, name)
	if err != nil {
		return "", err
	}
	switch {
	case rejected && container != "":
		return DirContainerRejected(container, task, feedback), nil
	case rejected:
		return DirRejected(task, feedback), nil
	case container != "":
		return DirContainerWorking(container, task), nil
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
	// A worker holding a feature is in the subtask loop — unless that feature has already landed.
	// Its PR being merged says so as plainly as its status does, and covers a feature left held by a
	// merge that took the partial-milestone path when it was in fact the last one.
	if st.Container != "" {
		if t, ok, _ := ps.GetTask(st.Container); ok && !featureLanded(ps, t) {
			switch st.Phase {
			case "submitted":
				feedback, rejected, err := e.prRejected(project, name)
				if err != nil {
					return "", err
				}
				if rejected {
					_ = ps.SetState(store.AgentState{Agent: name, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "working"})
					return DirContainerRejected(st.Container, st.Task, feedback), nil
				}
				return DirSubmitted, nil
			case "working":
				return e.workDirective(project, name, st.Task, st.Container)
			default:
				// A failure here is surfaced, never read as "finished": the feature is only done
				// when the store says there is nothing under it, not when assignment went wrong.
				next, ok, aerr := e.advanceContainer(project, name, st.Container)
				if aerr != nil {
					return "", aerr
				}
				if ok {
					return DirContainerWorking(st.Container, next.ID), nil
				}
				return DirContainerDone(st.Container), nil
			}
		}
		_ = ps.SetState(store.AgentState{Agent: name, Phase: "idle"})
		return e.waitForWork(ctx, func() (string, bool, error) { return e.claimNext(project, name) })
	}
	switch st.Phase {
	case "working":
		return e.workDirective(project, name, st.Task, "")
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
	// Before reading any source: a repo arriving with a td backlog gets it once, or its tasks
	// would simply be absent from the moment td stopped being a source.
	if err := e.importTdOnce(project, root); err != nil {
		return err
	}
	var rows []store.Task

	// Every source treated identically — the hub never branches on which it is. Each self-gates,
	// normalizes to task.Task, and throttles internally. td errors fail the sync (it is primary);
	// a network source degrades to its last good list.
	for _, src := range e.taskSources(project) {
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
	// Parentage is the hub's for every task, so it goes on after the sources rather than coming
	// from them: no source but sindri's own carries the notion at all.
	if links, err := ps.ParentLinks(); err == nil {
		for i := range rows {
			if parent, ok := links[rows[i].ID]; ok {
				rows[i].ParentID = parent
			}
		}
	}
	return ps.ReplaceTasks(rows)
}

// SetPriority assigns a task's priority (a P-code) in a project.
func (e *Engine) SetPriority(project, id, priority string) error {
	if ps := e.store.For(project); ps.OwnsTask(id) {
		if err := ps.SetOwnedPriority(id, priority); err != nil {
			return err
		}
	} else {
		if err := ps.SetPriorityOverride(id, priority); err != nil {
			return err
		}
	}
	e.refreshCachedTask(project, id) // targeted refresh of the reprioritized task
	e.deps.Notify()
	// Rating an unrated task is the moment it becomes claimable — a gh-* issue imported without
	// one, say — so it needs the same nudge as a task created with a priority.
	e.nudgeIdleWorkers(project, id, priority)
	return nil
}

// checkParent validates a requested parent before anything is written: it must exist, and it must
// not already sit below the task being re-parented. A loop is unreachable from any root, so the task
// list would simply stop showing every task inside it.
func (e *Engine) checkParent(project, parent, self string) error {
	if parent == "" {
		return nil
	}
	if parent == self {
		return fmt.Errorf("a task can't be its own parent")
	}
	ps := e.store.For(project)
	tasks, err := ps.AllTasks()
	if err != nil {
		return err
	}
	known := false
	for _, t := range tasks {
		if t.ID == parent {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("unknown parent %q", parent)
	}
	if self == "" {
		return nil // a task being created has nothing below it yet
	}
	// Walk up from the proposed parent, reading the links themselves rather than the read model
	// they are laid over. Reaching self means self is already an ancestor.
	links, err := ps.ParentLinks()
	if err != nil {
		return err
	}
	chain := []string{parent}
	for at := links[parent]; at != ""; at = links[at] {
		if at == self {
			return fmt.Errorf("%s already sits above %s (%s) — parenting it there would close a loop, "+
				"and everything inside a loop drops off the task list", self, parent,
				strings.Join(append(chain, self), " → "))
		}
		chain = append(chain, at)
		if len(chain) > len(links)+1 {
			return fmt.Errorf("the parent chain above %q doesn't terminate — a loop is already stored (%s)",
				parent, strings.Join(chain, " → "))
		}
	}
	return nil
}

// ToStoreTask maps a source-normalized domain task onto the hub's cached store row.
// Exported because the hub's targeted single-task refresh reuses the same mapping.
func ToStoreTask(t task.Task) store.Task {
	var updatedAt string
	if !t.UpdatedAt.IsZero() {
		updatedAt = t.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return store.Task{
		ID: t.ID, Title: t.Title, Status: t.Status, Priority: t.Priority,
		Type: t.Type, Labels: strings.Join(t.Labels, ","), ParentID: t.ParentID,
		Description: t.Description, URL: t.URL, UpdatedAt: updatedAt,
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
	// Only a task sindri owns carries a status of its own. A gh-* issue's "in_progress" lives in
	// agent_state (which OpenLeaves honours) — GitHub is told nothing until the merge closes it.
	if ps.OwnsTask(t.ID) {
		if err := ps.SetOwnedStatus(t.ID, "in_progress"); err != nil {
			return "", false, err
		}
		_ = e.RefreshTask(project, t.ID)
	}
	// Lay the new branch on a CLEAN base: leftover WIP from a cancelled task would block
	// `checkout -B` or bleed in. Reset at claim time, not at cancel — the agent may work on after
	// the push, and it never cleans its own worktree.
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
