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

// TaskInfo returns one task, refreshed from its source of truth. Only sindri's own tasks live in
// the hub's store as authoritative; a mirrored id is served from the cache, since asking the owned
// store by a foreign id only errors.
func (e *Engine) TaskInfo(project, id string) (store.Task, error) {
	if !task.IsOwned(id) {
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
	id, err := task.MintID()
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
		if st.Task == "" {
			continue
		}
		_ = e.SetStatus(a.Project, st.Task, "open")
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
	if err := e.SetStatus(project, id, "open"); err != nil {
		return err
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

// commentBlock renders a task's thread oldest-first, shaped like the CLI's `task info` so the two
// read alike. Source is on the head line: it says who else has already seen the comment.
func commentBlock(comments []store.Comment) string {
	var b strings.Builder
	for _, c := range comments {
		fmt.Fprintf(&b, "\n— %s (%s, %s)\n%s\n", dash(c.Author), c.Source, c.CreatedAt,
			strings.TrimRight(c.Body, "\n"))
	}
	return b.String()
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

// workDirective is what a working agent is told: a rejected PR's feedback is PUSHED every time it
// asks, so it never hunts for why the PR bounced; otherwise the plain "work on the task". container,
// the feature it holds, decides which verb the directive names, and MUST match what the registry shows
// that caller — a directive naming a hidden verb leaves the agent to improvise the workflow.
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
		return e.waitForWork(ctx, func() (string, bool, error) { return e.reviewDirective(project, name) })
	}
	st, _ := ps.GetState(name)
	if a.Role == "planner" {
		switch st.Phase {
		case "submitted":
			return DirSubmitted, nil
		case "planning": // set by AssignPlan and by `state planning` — it HAS work in hand
			return DirPlanning, nil
		}
		return DirPlanner, nil
	}
	// A worker holding a feature is in the subtask loop — unless that feature has already landed. A
	// merged PR says so as plainly as its status, and covers one left held by a partial-milestone merge.
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
		return e.waitForNextTask(ctx, project, name)
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
		return e.waitForNextTask(ctx, project, name)
	}
}

// waitForNextTask is the idle-agent path: an agent that will get no more work is told so
// immediately, not left blocking on a queue it is no longer served from.
func (e *Engine) waitForNextTask(ctx context.Context, project, name string) (string, error) {
	if a, ok, _ := e.store.For(project).GetAgent(name); ok && a.Retired {
		return DirRetired, nil
	}
	if tokens, full := e.contextFull(project, name); full {
		return DirFull(tokens), nil
	}
	return e.waitForWork(ctx, func() (string, bool, error) { return e.claimNext(project, name) })
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

// SetPriority assigns a task's priority (a P-code), reaching as far below it as scope asks. What
// reaching there does differs by case, and api.PriorityEffect is where that is set out.
func (e *Engine) SetPriority(project, id, priority string, scope api.PriorityScope) error {
	targets, err := e.priorityTargets(project, id, scope)
	if err != nil {
		return err
	}
	for _, t := range targets {
		if err := e.writePriority(project, t, priority); err != nil {
			return err
		}
		e.refreshCachedTask(project, t) // targeted refresh of each reprioritized task
	}
	e.deps.Notify()
	// Rating an unrated task is the moment it becomes claimable, so it needs the same nudge as a task
	// created with a priority. ONE, however far the cascade reached: it only has to wake a worker up.
	e.nudgeIdleWorkers(project, id, priority)
	return nil
}

// priorityTargets is which tasks a scoped rating writes to, CHILDREN FIRST — the parent's rating is
// what releases a package, so no worker can claim one half-rated. Open descendants only: a finished
// task's rating decides nothing, and overwriting it would edit the record of work already done.
func (e *Engine) priorityTargets(project, id string, scope api.PriorityScope) ([]string, error) {
	if scope == api.ScopeTask {
		return []string{id}, nil
	}
	all, err := e.store.For(project).AllTasks()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, d := range api.Descendants(all, id) {
		if !api.Open(d) || (scope == api.ScopeUnrated && d.Priority != "") {
			continue
		}
		out = append(out, d.ID)
	}
	return append(out, id), nil
}

// writePriority records one rating where that task's priority lives: its own row when sindri owns the
// task, the hub's overlay when the task is mirrored.
func (e *Engine) writePriority(project, id, priority string) error {
	ps := e.store.For(project)
	if ps.OwnsTask(id) {
		return ps.SetOwnedPriority(id, priority)
	}
	return ps.SetPriorityOverride(id, priority)
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
	var updatedAt, createdAt string
	if !t.UpdatedAt.IsZero() {
		updatedAt = t.UpdatedAt.UTC().Format(time.RFC3339)
	}
	// Left empty when the source has no answer, rather than stamped with now: the store keeps what it
	// already had, and inventing a time here would age every task from the last sync.
	if !t.CreatedAt.IsZero() {
		createdAt = t.CreatedAt.UTC().Format(time.RFC3339)
	}
	return store.Task{
		ID: t.ID, Title: t.Title, Status: t.Status, Priority: t.Priority,
		Type: t.Type, Labels: strings.Join(t.Labels, ","), ParentID: t.ParentID,
		Description: t.Description, URL: t.URL, UpdatedAt: updatedAt, CreatedAt: createdAt,
	}
}

// CmdNext claims the highest-priority open task for a worker and branches for it.
func (e *Engine) CmdNext(c registry.Caller, _ []string, out io.Writer) (int, error) {
	if a, ok, _ := e.store.For(c.Project).GetAgent(c.Agent); ok && a.Retired {
		fmt.Fprintln(out, DirRetired)
		return 0, nil
	}
	if tokens, full := e.contextFull(c.Project, c.Agent); full {
		fmt.Fprintln(out, DirFull(tokens))
		return 0, nil
	}
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

// ContextFullFraction is how much of its window a worker may fill before it stops being handed new
// work. A fraction, not a token count: a flat 170k written for a 200k window retired workers on a
// 1M one with most of it unused.
const ContextFullFraction = 0.85

// contextFull is the one fact both the assignment gate and the board's status read. No recorded
// usage yet (ok=false from ContextUsage) is never full, and neither is a window of 0 — an unknown
// window must not retire anybody, since guessing one is what this replaced.
func (e *Engine) contextFull(project, worker string) (tokens int, full bool) {
	tokens, window, ok := e.deps.ContextUsage(project, worker)
	if !ok || window <= 0 {
		return tokens, false
	}
	return tokens, float64(tokens) >= float64(window)*ContextFullFraction
}

// ContextFull is contextFull's bool half, for the board's status word.
func (e *Engine) ContextFull(project, worker string) bool {
	_, full := e.contextFull(project, worker)
	return full
}

// claimNext claims the highest-priority open LEAF task (or a marked container) for a worker in a
// project. Returns (directive, true) on a claim, ("", false) when idle or retired (full).
func (e *Engine) claimNext(project, agent string) (string, bool, error) {
	// Retired by a human, or by its own context filling: either way it is being wound down, and the
	// gate is here rather than at the task queries so it holds however the work would have arrived.
	if a, ok, _ := e.store.For(project).GetAgent(agent); ok && a.Retired {
		return "", false, nil
	}
	if _, full := e.contextFull(project, agent); full {
		return "", false, nil // retired: a full worker is not handed new work
	}
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
	if err := e.SetStatus(project, t.ID, "in_progress"); err != nil {
		return "", false, err
	}
	_ = e.RefreshTask(project, t.ID)
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
