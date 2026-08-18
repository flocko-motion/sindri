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
	"github.com/flo-at/sindri/internal/brokkr/lint"
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
	_ = ps.UpsertTask(ownedToCachedTask(owned, ps.ParentOf(id)))
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
		ID: id, Title: s.Title, Status: "open", Priority: s.Priority, Tier: s.Tier, Type: typ,
		Labels: strings.Join(s.Labels, ","), Description: s.Description,
	}); err != nil {
		return "", err
	}
	if err := ps.SetParent(id, s.Parent); err != nil {
		return "", err
	}
	e.refreshCachedTask(project, id) // targeted: pull just the new task, not a full re-sync
	e.adoptChild(project, s.Parent, id)
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
		if !e.deps.AgentUp(project, a.Name) {
			continue // nothing to inject into
		}
		_ = e.deps.Deliver(project, a.Name, MsgWorkAvailable(id), PushOnly)
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
	gained := false
	if s.Parent != "" {
		// Diff rather than echo: re-parenting a task to where it already sits adds no child, and
		// telling that parent's holder one arrived is noise about work it has had all along.
		gained = ps.ParentOf(id) != s.Parent
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
	// Re-parenting adds a child as surely as creating one does, so the same growth applies: whoever
	// is working the new parent takes this on too, rather than merging over it.
	if gained {
		e.adoptChild(project, s.Parent, id)
	}
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
	aim, ceiling := e.commentBudget(project)
	switch {
	case rejected && container != "":
		return DirContainerRejected(container, task, feedback, aim, ceiling), nil
	case rejected:
		return DirRejected(task, feedback, aim, ceiling), nil
	case container != "":
		return DirContainerWorking(container, task, aim, ceiling), nil
	}
	return DirWorking(task, aim, ceiling), nil
}

// commentBudget resolves the SAME two numbers the submit gate's comment-length trend checks
// against: the ceiling via lint.MaxCommentAvgFor — the one resolver cmd/brokkr's own flag layer
// also sits on top of, so the two can never drift apart — and the aim lint.AimFor derives from it.
func (e *Engine) commentBudget(project string) (aim, ceiling float64) {
	cfg, _ := e.deps.ProjectConfig(project) // unreadable: cfg is the zero value, which resolves the default
	ceiling = lint.MaxCommentAvgFor(cfg)
	return lint.AimFor(ceiling), ceiling
}

// pendingMail is the directive for unread mail, when there is any. Deferred by the task-boundary
// gates (claimNext, claimNextSubtask, reviewDirective's unclaimed path) until any pending clear,
// model change or compaction resolves — else mail is read into the context that operation discards.
func (e *Engine) pendingMail(project, name string) (dir string, has bool, err error) {
	n, err := e.store.For(project).UnreadMailCount(name)
	if err != nil {
		return "", false, err
	}
	return DirUnreadMail(n), n > 0, nil
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
	st, _ := ps.GetState(name)
	// Escalated outranks every role's directive — repeated on EVERY ask, since a relaunched agent has
	// no memory of asking. Mail outranks even that: it may answer or moot the question.
	if st.Escalation != "" {
		if d, has, err := e.pendingMail(project, name); err != nil {
			return "", err
		} else if has {
			return d, nil
		}
		return DirEscalated(st.Escalation), nil
	}
	if a.Role == "coauthor" {
		if d, has, err := e.pendingMail(project, name); err != nil {
			return "", err
		} else if has {
			return d, nil
		}
		return DirCoauthor, nil
	}
	if a.Role == "reviewer" { // reviewDirective checks its own mail, deferring it the same way
		return e.waitForWork(ctx, func() (string, bool, error) { return e.reviewDirective(project, name) })
	}
	if a.Role == "planner" {
		if d, has, err := e.pendingMail(project, name); err != nil {
			return "", err
		} else if has {
			return d, nil
		}
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
			case "submitted", "gating", "working":
				if d, has, err := e.pendingMail(project, name); err != nil {
					return "", err
				} else if has {
					return d, nil
				}
			}
			switch st.Phase {
			case "submitted":
				feedback, rejected, err := e.prRejected(project, name)
				if err != nil {
					return "", err
				}
				if rejected {
					_ = ps.SetState(store.AgentState{Agent: name, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "working"})
					aim, ceiling := e.commentBudget(project)
					return DirContainerRejected(st.Container, st.Task, feedback, aim, ceiling), nil
				}
				return DirSubmitted, nil
			case "gating":
				return DirGating, nil
			case "working":
				return e.workDirective(project, name, st.Task, st.Container)
			default:
				// Blocking: a feature whose remaining work is gated waits like any other empty queue.
				// claimNextSubtask defers mail past whatever clear or compaction it resolves here.
				return e.waitForWork(ctx, func() (string, bool, error) {
					if fired, err := e.fireClearIfArmed(project, name); err != nil {
						return "", false, err
					} else if fired {
						return DirClearPending, true, nil
					}
					return e.claimNextSubtask(project, name, st.Container)
				})
			}
		}
		_ = ps.SetState(store.AgentState{Agent: name, Phase: "idle"})
		return e.waitForNextTask(ctx, project, name)
	}
	switch st.Phase {
	case "working", "submitted", "gating":
		if d, has, err := e.pendingMail(project, name); err != nil {
			return "", err
		} else if has {
			return d, nil
		}
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
			aim, ceiling := e.commentBudget(project)
			return DirRejected(st.Task, feedback, aim, ceiling), nil
		}
		return DirSubmitted, nil
	case "gating":
		return DirGating, nil
	default: // idle — claim the next task; claimNext defers mail past whatever it resolves for it.
		return e.waitForNextTask(ctx, project, name)
	}
}

// retired reports a human-parked agent — hands off every automatic behaviour, written once so a
// feature added later asks this instead of keeping its own copy (-> retire.go).
func (e *Engine) retired(project, name string) bool {
	a, ok, err := e.store.For(project).GetAgent(name)
	return err == nil && ok && a.Retired
}

// waitForNextTask is the idle-agent path. An armed clear fires on every check regardless of
// whether work exists; compact and model-select are claimNext's, once it has an assignment.
func (e *Engine) waitForNextTask(ctx context.Context, project, name string) (string, error) {
	if e.retired(project, name) {
		// Told to sit still, same as an escalation — mail outranks it for the same reason.
		if d, has, err := e.pendingMail(project, name); err != nil {
			return "", err
		} else if has {
			return d, nil
		}
		return DirRetired, nil
	}
	return e.waitForWork(ctx, func() (string, bool, error) {
		if fired, err := e.fireClearIfArmed(project, name); err != nil {
			return "", false, err
		} else if fired {
			return DirClearPending, true, nil
		}
		if tokens, full := e.contextFull(project, name); full {
			// Also told to sit still, not a pending operation for mail to wait out.
			if d, has, err := e.pendingMail(project, name); err != nil {
				return "", false, err
			} else if has {
				return d, true, nil
			}
			return DirFull(tokens), true, nil
		}
		return e.claimNext(project, name)
	})
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

// CmdNext claims the highest-priority open task for a worker and branches for it.
func (e *Engine) CmdNext(c registry.Caller, _ []string, out io.Writer) (int, error) {
	if e.retired(c.Project, c.Agent) {
		fmt.Fprintln(out, DirRetired)
		return 0, nil
	}
	if fired, err := e.fireClearIfArmed(c.Project, c.Agent); err != nil {
		return 1, err
	} else if fired {
		fmt.Fprintln(out, DirClearPending)
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

// claimNext hands a worker the best-rated unit in a project (-> nextUp), preparing it first: a model
// change or compaction ends this pass right there, same as clearArmed above — mail and handover only
// run once neither fires, so the assignment always lands in the context that follows either one.
func (e *Engine) claimNext(project, agent string) (string, bool, error) {
	// Retired by a human, or by its own context filling: either way it is being wound down, and the
	// gate is here rather than at the task queries so it holds however the work would have arrived.
	if e.retired(project, agent) {
		return "", false, nil
	}
	if e.clearArmed(project, agent) {
		return "", false, nil // about to land (fired by the caller): work claimed now would be cut in half by it
	}
	if _, full := e.contextFull(project, agent); full {
		return "", false, nil
	}
	_ = e.SyncTasks(project) // best-effort refresh; cached set on failure
	ps := e.store.For(project)
	packages, err := ps.OpenContainers()
	if err != nil {
		return "", false, err
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		return "", false, err
	}
	t, isPackage, ok := nextUp(packages, leaves, e.tierPrefers(project, agent))
	if ok {
		tier := api.TierOrDefault(t.Tier)
		if want, known := e.deps.ModelForTier(tier); known && want != e.deps.CurrentModel(project, agent) {
			if err := e.deps.SetModel(project, agent, want); err != nil {
				return "", false, err
			}
			return DirRetiering(tier), true, nil
		}
		if dir, acted, err := e.compactOrWait(project, agent); err != nil {
			return "", false, err
		} else if acted {
			return dir, true, nil
		}
	}
	if d, has, err := e.pendingMail(project, agent); err != nil { // neither op above fired; safe to check now
		return "", false, err
	} else if has {
		return d, true, nil
	}
	if !ok {
		return "", false, nil
	}
	if isPackage {
		return e.claimContainer(project, agent, t)
	}
	return e.claimLeaf(project, agent, t)
}

// claimLeaf claims one standalone task for a worker, branching on it.
func (e *Engine) claimLeaf(project, worker string, t store.Task) (string, bool, error) {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
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
	// A claim is what earns the right to speak to the user, so the note grant is given here and
	// REPLACES whatever was left (-> store.GrantNotes).
	if err := ps.GrantNotes(worker, NotesPerClaim); err != nil {
		return "", false, err
	}
	if err := ps.SetState(store.AgentState{Agent: worker, Task: t.ID, Branch: branch, Phase: "working"}); err != nil {
		return "", false, err
	}
	_ = ps.Log(worker, "claim", t.ID+" "+t.Title)
	e.deps.Notify()
	return DirClaimed(t.ID, t.Title, branch, e.deps.ArchitectureDoc(project)), true, nil
}
