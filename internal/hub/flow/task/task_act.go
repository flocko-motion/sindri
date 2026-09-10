// package: hub/flow/task / task_act
// type:    logic (the act → report → idle loop + PR-as-merge-intent)
// job:     the worker verbs and task assignment. Tasks are a cached read model
// synced from td (D15); `next` claims one and branches; the directive loop
// decides the next action. All state is per-project — methods take a
// project (repoTag) and work through store.For(project).
// limits:  git is entirely hub-side (the agent edits /workspace, the hub commits
// and merges); writes to td go through the td adapter (D15).
package task

import (
	"context"
	"fmt"
	"github.com/flo-at/sindri/internal/hub/core"
	ownedpkg "github.com/flo-at/sindri/internal/hub/owned"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"io"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/brokkr/lint"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// Tasks refreshes from td and returns all cached tasks for a project (for `task
// list`). A sync failure is surfaced, never swallowed.
func (a *Act) Tasks(project string) ([]store.Task, error) {
	if err := a.SyncTasks(project); err != nil {
		return nil, err
	}
	// Repair any stale status (in_review with no PR, in_progress with no assignee)
	// against reality — a listing is a natural, infrequent point to do the sweep.
	_ = a.ReconcileTasks(project)
	return a.Store.For(project).AllTasks()
}

// TaskInfo returns one task, refreshed from its source of truth: sindri's own from the store, a
// mirrored id from the cache (the store errors on a foreign id).
func (a *Act) TaskInfo(project, id string) (store.Task, error) {
	if !task.IsOwned(id) {
		t, ok, err := a.Store.For(project).GetTask(id)
		if err != nil {
			return store.Task{}, err
		}
		if !ok {
			return store.Task{}, fmt.Errorf("%w %q", core.ErrNoSuchTask, id)
		}
		t.Comments = a.Deps.TaskComments(project, id)
		return t, nil
	}
	// Repair this one task's status against reality before returning it (task info /
	// detail is a natural single-task check point).
	_ = a.ReconcileTask(project, id)
	ps := a.Store.For(project)
	owned, ok, err := ps.OwnedTask(id)
	if err != nil {
		return store.Task{}, err
	}
	if !ok {
		return store.Task{}, fmt.Errorf("%w %q", core.ErrNoSuchTask, id)
	}
	_ = ps.UpsertTask(ownedToCachedTask(owned, ps.ParentOf(id)))
	// Read the row back rather than returning what was just written: the approval gate lives in its
	// own table and reaches a task only through that join, so a hand-built row reports none.
	st, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		return store.Task{}, err
	}
	st.Comments = a.Deps.TaskComments(project, id)
	return st, nil
}

// TaskSpec is the full editable shape of a task, the payload of both create and edit — it crosses
// the wire, so it is internal/api.TaskSpec under the name every existing caller here already uses.
type TaskSpec = api.TaskSpec

// CreateTask creates a task via the td tool in a project and returns its id.
func (a *Act) CreateTask(project string, s TaskSpec) (string, error) {
	if err := checkTier(s.Tier); err != nil {
		return "", err
	}
	if err := a.checkParent(project, s.Parent, ""); err != nil {
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
	ps := a.Store.For(project)
	if err := ps.PutOwnedTask(store.OwnedTask{
		ID: id, Title: s.Title, Status: "open", Priority: s.Priority, Tier: s.Tier, Type: typ,
		Labels: strings.Join(s.Labels, ","), Description: s.Description,
	}); err != nil {
		return "", err
	}
	if err := ps.SetParent(id, s.Parent); err != nil {
		return "", err
	}
	a.RefreshCachedTask(project, id) // targeted: pull just the new task, not a full re-sync
	a.AdoptChild(project, s.Parent, id)
	a.Deps.Notify()
	a.Flow.WakeProject(project, topic.TaskAvailable) // a new task may be the next one for any of them
	return id, nil
}

// HealPlannerTasks releases any backlog task a planner is holding — an invalid
// assignment. Self-heals stale claims; runs once at hub boot, across all projects.
func (a *Act) HealPlannerTasks() {
	agents, _ := a.Store.AllAgents()
	for _, ag := range agents {
		if ag.Role != "planner" {
			continue
		}
		ps := a.Store.For(ag.Project)
		st, _ := ps.GetState(ag.Name)
		if st.Task == "" {
			continue
		}
		_ = a.pr().SetStatus(ag.Project, st.Task, "open")
		_ = ps.SetState(store.AgentState{Agent: ag.Name, Phase: "planning"}, store.ReasonFreed, "planners don't hold tasks: "+st.Task)
		_ = ps.Log(ag.Name, "unassign", st.Task+" (planners don't hold tasks)")
	}
}

// UnassignTask releases a task in a project back to the backlog and clears it from
// whatever agent held it. Refused if that agent is currently alive and working.
func (a *Act) UnassignTask(project, id string) error {
	ps := a.Store.For(project)
	roster, _ := ps.Roster()
	for _, ag := range roster {
		st, _ := ps.GetState(ag.Name)
		if st.Task != id {
			continue
		}
		if a.Harness.Probe(project, ag.Name).Up {
			return fmt.Errorf("%s is alive and working on %s — stop or delete it first", ag.Name, id)
		}
		// A container holder rests back onto its FEATURE, not fully idle: unassigning one subtask
		// does not mean the feature it lives under is done (sd-5ef393 — the same shape as
		// FinishTask's own fix).
		next := store.AgentState{Agent: ag.Name, Phase: "idle"}
		if st.Container != "" {
			next = store.AgentState{Agent: ag.Name, Container: st.Container, Branch: st.Container, Phase: "idle"}
		}
		_ = ps.SetState(next, store.ReasonFreed, "unassigned: "+id)
		_ = ps.Log(ag.Name, "unassign", id)
	}
	if err := a.pr().SetStatus(project, id, "open"); err != nil {
		return err
	}
	_ = a.RefreshTask(project, id)
	a.Deps.Notify()
	return nil
}

// The approval gate (approve/reject) lives in approve_act.go; the planner's verbs in planner_act.go.

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
func (a *Act) EditTask(project, id string, s TaskSpec) error {
	if err := checkTier(s.Tier); err != nil {
		return err
	}
	if err := a.checkParent(project, s.Parent, id); err != nil {
		return err
	}
	ps := a.Store.For(project)
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
		ownedpkg.ApplySpec(&owned, s)
		if err := ps.PutOwnedTask(owned); err != nil {
			return err
		}
	} else {
		// A mirrored task's CONTENT belongs to its source; priority and tier do not — no source
		// carries either, so both are sindri's to assign on any task. Tier was dropped here, which
		// left every openspec change stuck at the default and handed to a mid-tier model.
		if s.Priority != "" {
			if err := ps.SetPriorityOverride(id, s.Priority); err != nil {
				return err
			}
		}
		if s.Tier != "" {
			if err := ps.SetTierOverride(id, s.Tier); err != nil {
				return err
			}
		}
	}
	a.RefreshCachedTask(project, id) // targeted refresh of the edited task
	// Re-parenting adds a child as surely as creating one does, so the same growth applies: whoever
	// is working the new parent takes this on too, rather than merging over it.
	if gained {
		a.AdoptChild(project, s.Parent, id)
	}
	a.Deps.Notify()
	return nil
}

// PrRejected reports a rejected PR for the work IN HAND and its feedback, so the worker is handed the
// comments directly. Scoped to target because matching any rejected PR by this author served an old
// one for ever: an agent was told its current task was rejected, over feedback about a finished one.
func (a *Act) PrRejected(project, agent, target string) (feedback string, rejected bool, err error) {
	if target == "" {
		return "", false, nil // nothing held, so no rejection of it to report
	}
	prs, err := a.Store.For(project).PRs()
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

// RejectionRound is how many times this PR has come back, 1 for the first. Told to the author
// because the count is the fact that should change its approach: 171 of the fleet's 409 submissions
// were rejected, one of them nine times, each round costing a gate run and an exhaustive read.
func (a *Act) RejectionRound(project, target string) int {
	evs, err := a.Store.For(project).PREvents("pr-" + target)
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

// CommentBudget resolves the SAME two numbers the submit gate's own trend check uses — the ceiling
// via lint.MaxCommentAvgFor, and the aim lint.AimFor derives from it — so the two can never drift apart.
func (a *Act) CommentBudget(project string) (aim, ceiling float64) {
	cfg, _ := a.Deps.ProjectConfig(project) // unreadable: cfg is the zero value, which resolves the default
	ceiling = lint.MaxCommentAvgFor(cfg)
	return lint.AimFor(ceiling), ceiling
}

// CmdNext claims the highest-priority open task for a worker and branches for it.
func (a *Act) CmdNext(c registry.Caller, _ []string, out io.Writer) (int, error) {
	if a.Retired(c.Project, c.Agent) {
		fmt.Fprintln(out, prompts.DirRetired)
		return 0, nil
	}
	if fired, err := a.roles().FireClearIfArmed(c.Ctx, c.Project, c.Agent); err != nil {
		return 1, err
	} else if fired {
		fmt.Fprintln(out, prompts.DirBusy("worker/clearing"))
		return 0, nil
	}
	preamble, err := a.roles().ServeMail(c.Project, c.Agent)
	if err != nil {
		return 1, err
	}
	fmt.Fprint(out, preamble)
	d, claimed, err := a.ClaimNext(c.Ctx, c.Project, c.Agent)
	if err != nil {
		return 1, err
	}
	if !claimed {
		fmt.Fprintln(out, prompts.DirNoTasks)
		return 0, nil
	}
	fmt.Fprintln(out, d)
	return 0, nil
}

// ClaimNext hands a worker the best-rated unit in a project (-> nextUp): the claim comes FIRST, so
// holding the work protects it while preparation (a model switch, else a clear) runs.
func (a *Act) ClaimNext(ctx context.Context, project, agent string) (string, bool, error) {
	// Retired by a human: wound down deliberately, and the gate is here rather than at the task
	// queries so it holds however the work would have arrived.
	if a.Retired(project, agent) {
		return "", false, nil
	}
	if a.ClearArmedFor(project, agent) {
		return "", false, nil // about to land (fired by the caller): work claimed now would be cut in half by it
	}
	_ = a.SyncTasks(project) // best-effort refresh; cached set on failure
	ps := a.Store.For(project)
	packages, err := ps.OpenContainers()
	if err != nil {
		return "", false, err
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		return "", false, err
	}
	t, isPackage, ok := task.NextUp(packages, leaves, a.roles().TierPrefers(project, agent))
	if !ok {
		return "", false, nil
	}
	var dir string
	if isPackage {
		dir, _, err = a.ClaimContainer(project, agent, t)
	} else {
		dir, _, err = a.ClaimLeaf(project, agent, t)
	}
	if err != nil {
		return "", false, err
	}
	// The memory means "told about this while idle, and it did not claim" — a claim, whichever task it
	// lands on, ends that, or the next time this same task comes back around nobody hears about it.
	_ = ps.SetLastNudge(agent, "")
	if _, err := a.roles().PrepareAssignment(ctx, project, agent, api.TierOrDefault(t.Tier), dir); err != nil {
		return "", false, err
	}
	return dir, true, nil
}

// ClaimLeaf claims one standalone task for a worker, branching on it.
func (a *Act) ClaimLeaf(project, worker string, t store.Task) (string, bool, error) {
	ps := a.Store.For(project)
	root := a.Deps.ProjectRoot(project)
	base, err := a.BaseBranch(root)
	if err != nil {
		return "", false, err
	}
	ag, ok, err := ps.GetAgent(worker)
	if err != nil || !ok {
		return "", false, fmt.Errorf("agent %s missing: %v", worker, err)
	}
	wt := filepath.Join(root, ag.Workspace)
	branch := t.ID
	if err := a.pr().SetStatus(project, t.ID, "in_progress"); err != nil {
		return "", false, err
	}
	_ = a.RefreshTask(project, t.ID)
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
	if err := ps.GrantNotes(worker, prompts.NotesPerClaim); err != nil {
		return "", false, err
	}
	if err := ps.SetState(store.AgentState{Agent: worker, Task: t.ID, Branch: branch, Phase: "working"},
		store.ReasonClaimed, "claimed "+t.ID); err != nil {
		return "", false, err
	}
	_ = ps.Log(worker, "claim", t.ID+" "+t.Title)
	a.Deps.Notify()
	return prompts.DirClaimed(t.ID, t.Title, branch, a.Deps.ArchitectureDoc(project)), true, nil
}
