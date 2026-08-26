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

// TaskInfo returns one task, refreshed from its source of truth: sindri's own from the store, a
// mirrored id from the cache (the store errors on a foreign id).
func (e *Engine) TaskInfo(project, id string) (store.Task, error) {
	if !task.IsOwned(id) {
		t, ok, err := e.store.For(project).GetTask(id)
		if err != nil {
			return store.Task{}, err
		}
		if !ok {
			return store.Task{}, fmt.Errorf("%w %q", ErrNoSuchTask, id)
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
		return store.Task{}, fmt.Errorf("%w %q", ErrNoSuchTask, id)
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

// TaskSpec is the full editable shape of a task, the payload of both create and edit — it crosses
// the wire, so it is internal/api.TaskSpec under the name every existing caller here already uses.
type TaskSpec = api.TaskSpec

// CreateTask creates a task via the td tool in a project and returns its id.
func (e *Engine) CreateTask(project string, s TaskSpec) (string, error) {
	if err := checkTier(s.Tier); err != nil {
		return "", err
	}
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
	e.nudgeIdleWorkers(project, s.Priority)
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
	if err := checkTier(s.Tier); err != nil {
		return err
	}
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

// prRejected reports a rejected PR for the work IN HAND and its feedback, so the worker is handed the
// comments directly. Scoped to target because matching any rejected PR by this author served an old
// one for ever: an agent was told its current task was rejected, over feedback about a finished one.
func (e *Engine) prRejected(project, agent, target string) (feedback string, rejected bool, err error) {
	if target == "" {
		return "", false, nil // nothing held, so no rejection of it to report
	}
	prs, err := e.store.For(project).PRs()
	if err != nil {
		return "", false, fmt.Errorf("load PRs for %s: %w", agent, err)
	}
	for _, p := range prs {
		if p.Agent == agent && p.Status == "rejected" && p.Task == target {
			return p.Feedback, true, nil
		}
	}
	return "", false, nil
}

// rejectionRound is how many times this PR has come back, 1 for the first. Told to the author
// because the count is the fact that should change its approach: 171 of the fleet's 409 submissions
// were rejected, one of them nine times, each round costing a gate run and an exhaustive read.
func (e *Engine) rejectionRound(project, target string) int {
	evs, err := e.store.For(project).PREvents("pr-" + target)
	if err != nil {
		return 1
	}
	n := 0
	for _, ev := range evs {
		if ev.Type == "rejected" {
			n++
		}
	}
	if n == 0 {
		return 1
	}
	return n
}

// workDirective is what a working agent is told: a rejected PR's feedback is PUSHED every ask, else the
// plain "work on the task". container, the feature it holds, must match what the registry shows that
// caller — a directive naming a hidden verb leaves the agent to improvise the workflow.
func (e *Engine) workDirective(project, name, task, container string) (string, error) {
	target := container // a feature's PR is filed against the feature, not the subtask in hand
	if target == "" {
		target = task
	}
	feedback, rejected, err := e.prRejected(project, name, target)
	if err != nil {
		return "", err
	}
	aim, ceiling := e.commentBudget(project)
	switch {
	case rejected && container != "":
		return DirContainerRejected(container, task, feedback, e.rejectionRound(project, target), aim, ceiling), nil
	case rejected:
		return DirRejected(task, feedback, e.rejectionRound(project, target), aim, ceiling), nil
	case container != "":
		return DirContainerWorking(container, task, aim, ceiling), nil
	}
	return DirWorking(task, aim, ceiling), nil
}

// commentBudget resolves the SAME two numbers the submit gate's own trend check uses — the ceiling
// via lint.MaxCommentAvgFor, and the aim lint.AimFor derives from it — so the two can never drift apart.
func (e *Engine) commentBudget(project string) (aim, ceiling float64) {
	cfg, _ := e.deps.ProjectConfig(project) // unreadable: cfg is the zero value, which resolves the default
	ceiling = lint.MaxCommentAvgFor(cfg)
	return lint.AimFor(ceiling), ceiling
}

// AgentDirective is the no-arg `sindri` answer, returned AT ONCE, mail served inline ahead of it
// (-> serveMail) unless a clear or a model switch lands this round instead.
func (e *Engine) AgentDirective(ctx context.Context, project, name string) (string, error) {
	beforeModel := e.deps.CurrentModel(project, name)
	dir, err := e.directive(ctx, project, name)
	if err != nil {
		return "", err
	}
	// A model switch narrates and restarts the agent inline, with no distinct text to spot in dir —
	// only the model actually changing under this call says so.
	retiered := e.deps.CurrentModel(project, name) != beforeModel
	if mailDeferred(dir) || retiered {
		return dir, nil
	}
	preamble, err := e.serveMail(project, name)
	if err != nil {
		return "", err
	}
	return preamble + dir, nil
}

// mailDeferred: DirClearPending discards this round's context, so mail waits for one that survives.
// DirPreparing is the same wait: the answer follows behind whatever fired (a clear, a compaction, or
// a model switch), so this reply is not the fresh context either.
func mailDeferred(dir string) bool {
	return dir == DirClearPending || dir == DirPreparing
}

// directive is the per-role/per-phase dispatch, mail lifted out to its one caller so no branch here
// has to stitch its own copy of that bookkeeping in.
func (e *Engine) directive(ctx context.Context, project, name string) (string, error) {
	ps := e.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("unknown agent %q", name)
	}
	// Asked BEFORE the state is read: an agent asking what to do is the hub's chance to notice two of
	// them in one tree, and the answer it would otherwise give is the wrong one — sudri was told its
	// feature was finished while dvalin worked the subtask holding it open.
	e.healSplit(project, name)
	st, _ := ps.GetState(name)
	// Escalated outranks every role's directive — repeated on EVERY ask, since a relaunched agent has
	// no memory of asking.
	if st.Escalation != "" {
		return DirEscalated(st.Escalation), nil
	}
	if a.Role == "coauthor" {
		return DirCoauthor, nil
	}
	if a.Role == "reviewer" {
		d, _, err := e.reviewDirective(project, name)
		return d, err
	}
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
				feedback, rejected, err := e.prRejected(project, name, st.Container)
				if err != nil {
					return "", err
				}
				if rejected {
					_ = ps.SetState(store.AgentState{Agent: name, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "working"})
					aim, ceiling := e.commentBudget(project)
					return DirContainerRejected(st.Container, st.Task, feedback,
						e.rejectionRound(project, st.Container), aim, ceiling), nil
				}
				return DirSubmitted, nil
			case "gating":
				return DirGating, nil
			case "working":
				return e.workDirective(project, name, st.Task, st.Container)
			default:
				if fired, err := e.fireClearIfArmed(project, name); err != nil {
					return "", err
				} else if fired {
					return DirClearPending, nil
				}
				d, _, err := e.claimNextSubtask(project, name, st.Container)
				return d, err
			}
		}
		_ = ps.SetState(store.AgentState{Agent: name, Phase: "idle"})
		return e.waitForNextTask(ctx, project, name)
	}
	switch st.Phase {
	case "working":
		return e.workDirective(project, name, st.Task, "")
	case "submitted":
		feedback, rejected, err := e.prRejected(project, name, st.Task)
		if err != nil {
			return "", err
		}
		if rejected {
			_ = ps.SetState(store.AgentState{Agent: name, Task: st.Task, Branch: st.Branch, Phase: "working"})
			aim, ceiling := e.commentBudget(project)
			return DirRejected(st.Task, feedback, e.rejectionRound(project, st.Task), aim, ceiling), nil
		}
		return DirSubmitted, nil
	case "gating":
		return DirGating, nil
	default: // idle — claim the next task, unless a PR of its own is still to land
		// Without this an agent whose state row was cleared out from under it (a checkpoint over an
		// unlanded PR) waits here for work agentBlocked refuses it over that very PR. austri sat
		// idle on a rejected pr-sd-a47b61, each side waiting for the other.
		if pr, task, aerr := ps.AwaitingPR(name); aerr == nil && pr != "" {
			p, ok, perr := ps.GetPR(pr)
			if perr == nil && ok && p.Status == "rejected" {
				aim, ceiling := e.commentBudget(project)
				return DirRejected(task, p.Feedback, e.rejectionRound(project, task), aim, ceiling), nil
			}
			return DirSubmitted, nil
		}
		return e.waitForNextTask(ctx, project, name)
	}
}

// serveMail fetches an agent's unread mail, marks it read, and renders it as a directive preamble —
// "" when there is none.
func (e *Engine) serveMail(project, name string) (string, error) {
	ps := e.store.For(project)
	msgs, err := ps.UnreadMail(name)
	if err != nil || len(msgs) == 0 {
		return "", err
	}
	for _, m := range msgs {
		if err := ps.MarkMailRead(m.ID); err != nil {
			return "", err
		}
	}
	e.deps.Notify() // the unread count is on the board
	return DirMail(msgs), nil
}

// retired reports a human-parked agent — hands off every automatic behaviour, written once so a
// feature added later asks this instead of keeping its own copy (-> retire.go).
func (e *Engine) retired(project, name string) bool {
	a, ok, err := e.store.For(project).GetAgent(name)
	return err == nil && ok && a.Retired
}

// waitForNextTask is the idle-agent path — a name kept from when this blocked; it now answers at
// once, and AssignPendingWork is what pushes a wake once there is something to claim.
func (e *Engine) waitForNextTask(ctx context.Context, project, name string) (string, error) {
	if e.retired(project, name) {
		return DirRetired, nil
	}
	if fired, err := e.fireClearIfArmed(project, name); err != nil {
		return "", err
	} else if fired {
		return DirClearPending, nil
	}
	d, claimed, err := e.claimNext(project, name)
	if err != nil {
		return "", err
	}
	if !claimed {
		return DirNoTasks, nil
	}
	return d, nil
}

// CmdNext claims the highest-priority open task for a worker and branches for it.
func (e *Engine) CmdNext(c registry.Caller, _ []string, out io.Writer) (int, error) {
	if e.retired(c.Project, c.Agent) {
		fmt.Fprintln(out, DirRetired)
		return 0, nil
	}
	if _, err := e.fireClearIfArmed(c.Project, c.Agent); err != nil {
		return 1, err
	}
	preamble, err := e.serveMail(c.Project, c.Agent)
	if err != nil {
		return 1, err
	}
	fmt.Fprint(out, preamble)
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

// claimNext hands a worker the best-rated unit in a project (-> nextUp): the claim comes FIRST, so
// holding the work protects it while preparation (a model switch, a clear, or compaction) runs.
func (e *Engine) claimNext(project, agent string) (string, bool, error) {
	// Retired by a human: wound down deliberately, and the gate is here rather than at the task
	// queries so it holds however the work would have arrived.
	if e.retired(project, agent) {
		return "", false, nil
	}
	if e.clearArmed(project, agent) {
		return "", false, nil // about to land (fired by the caller): work claimed now would be cut in half by it
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
	if !ok {
		return "", false, nil
	}
	var dir string
	if isPackage {
		dir, _, err = e.claimContainer(project, agent, t)
	} else {
		dir, _, err = e.claimLeaf(project, agent, t)
	}
	if err != nil {
		return "", false, err
	}
	// The memory means "told about this while idle, and it did not claim" — a claim, whichever task it
	// lands on, ends that, or the next time this same task comes back around nobody hears about it.
	_ = ps.SetLastNudge(agent, "")
	fired, err := e.prepareAssignment(project, agent, api.TierOrDefault(t.Tier), dir)
	if err != nil {
		return "", false, err
	}
	if fired {
		return DirPreparing, true, nil
	}
	return dir, true, nil
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
	// Lay the new branch on a CLEAN base: leftover WIP from a cancelled task would bleed in.
	// Reset at claim time, not at cancel — the agent may work on after the push.
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
