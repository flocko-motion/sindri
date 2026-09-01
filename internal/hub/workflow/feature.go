// package: hub/workflow / feature
// type:    logic (the subtask loop inside one feature branch)
// job:     working a task that has subtasks: claim the whole tree, hand out its open
// leaves one at a time on a single branch, checkpoint each, and close every
// parent the last of its children completes. The branch goes up as one PR
// (-> pr.go CmdSubmit) when nothing open is left under the feature.
// limits:  assignment and closure within a held tree; the leaf-task path and the
// directive loop are task.go's, and the PR is pr.go's.
package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// claimContainer assigns one package, starting its first open subtask — or, with none left,
// holding it so the agent finishes on the SAME branch (git.EnsureBranch), never a fresh one.
func (e *Engine) claimContainer(project, worker string, c store.Task) (string, bool, error) {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	children, err := ps.OpenSubtasks(c.ID)
	if err != nil {
		return "", false, err
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
	if len(children) == 0 {
		if err := ps.SetState(store.AgentState{Agent: worker, Container: c.ID, Branch: c.ID, Phase: "idle"},
			store.ReasonClaimed, "claimed container "+c.ID+" with nothing open under it"); err != nil {
			return "", false, err
		}
		_ = ps.Log(worker, "claim-container", c.ID+" "+c.Title+" (nothing left — finishing it)")
		return DirContainerDone(c.ID), true, nil
	}
	child := children[0]
	if err := e.startSubtask(project, worker, c.ID, child); err != nil {
		return "", false, err
	}
	_ = ps.Log(worker, "claim-container", c.ID+" "+c.Title)
	return DirContainerClaimed(c.ID, c.Title, child.ID, child.Title), true, nil
}

// CmdCheckpoint commits the current subtask to the feature branch, closes that
// subtask, and advances to the next — staying working, never blocking for review.
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
	// A rejection returns the work in phase "working", which is exactly the shape a checkpoint takes
	// for finished. austri checkpointed past a rejected pr-sd-a47b61, closing the task and freeing
	// itself for a second one it then had no room for.
	if pr, task, perr := ps.AwaitingPR(c.Agent); perr != nil {
		return 1, perr
	} else if pr != "" && task == st.Task {
		fmt.Fprintln(out, ReplyPRStillToLand(st.Task, pr))
		return 1, nil
	}
	a, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, a.Workspace)
	tk, _, _ := ps.GetTask(st.Task)
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		msg = tk.Title
	}
	if msg == "" {
		msg = "work on " + st.Task
	}
	msg = conventionalCommit(tk.Type, st.Task, msg)
	// A task with work under it can't be CLOSED by a checkpoint (an epic was once closed over four
	// open children) — it records progress and hands over the next leaf instead.
	grew, oerr := ps.OpenChildIDs(st.Task)
	if oerr != nil {
		return 1, oerr
	}
	if len(grew) == 0 {
		// The agent's WORKTREE, BEFORE the commit: a status living in the repo (an openspec change's
		// ticked boxes) exists only on its branch, so the hub's root read 0/10 for finished work and
		// handed the subtask back. Ended here it rides the commit and lands with the merge.
		if err := e.finishAtSource(c.Project, wt, st.Task, false); err != nil {
			return 1, err
		}
		// The step this path skipped by calling the source directly: without it advanceContainer
		// below re-picks the subtask just finished (-> recordEnded).
		if err := e.recordEnded(c.Project, st.Task); err != nil {
			return 1, err
		}
	}
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	if len(grew) == 0 {
		e.settleWithTask(c.Project, st.Task)
		_ = e.RefreshTask(c.Project, st.Task)
		e.closeCompletedAncestors(c.Project, st.Task, st.Container)
	}
	_ = ps.Log(c.Agent, "checkpoint", st.Task)
	done := st.Task
	// A checkpoint IS a leaf boundary, so an armed clear takes precedence over the next subtask:
	// the agent goes idle holding the feature, and the clear fires before anything else is served.
	if e.clearArmed(c.Project, c.Agent) {
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Container: st.Container, Branch: st.Container, Phase: "idle"},
			store.ReasonFreed, "checkpointed "+done+" with a clear armed")
		e.deps.Notify()
		fmt.Fprintln(out, ReplyCheckpointedClearing(done, st.Container))
		return 0, nil
	}
	next, ok, err := e.advanceContainer(c.Project, c.Agent, st.Container)
	if err != nil {
		return 1, err
	}
	if ok {
		if len(grew) > 0 {
			fmt.Fprintln(out, ReplyCheckpointedParentOpen(done, grew, next.ID, next.Title))
			return 0, nil
		}
		fmt.Fprintln(out, ReplyCheckpointed(done, next.ID, next.Title))
		return 0, nil
	}
	gated, err := e.gatedUnder(c.Project, st.Container)
	if err != nil {
		return 1, err
	}
	_ = ps.SetState(store.AgentState{Agent: c.Agent, Container: st.Container, Branch: st.Container, Phase: "idle"},
		store.ReasonAdvanced, "checkpointed "+done+", nothing left to claim under "+st.Container)
	e.deps.Notify()
	if len(gated) > 0 {
		fmt.Fprintf(out, "Checkpointed %s. %s\n", done, ReplyFeatureGated(st.Container, openIDs(gated)))
		return 0, nil
	}
	fmt.Fprintln(out, ReplyCheckpointedLast(done, st.Container))
	return 0, nil
}

// closeCompletedAncestors closes each parent above a just-closed task whose children are now all
// closed, stopping below stopAt (the feature itself, which its PR closes) — the only thing that
// marks an intermediate epic finished, or it stays open and is handed out as work that isn't there.
func (e *Engine) closeCompletedAncestors(project, from, stopAt string) {
	ps := e.store.For(project)
	for parent := ps.ParentOf(from); parent != "" && parent != stopAt; parent = ps.ParentOf(parent) {
		open, err := ps.OpenChildIDs(parent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hub: reading children of %s: %v\n", parent, err)
			return
		}
		if len(open) > 0 {
			return // work remains under it, so it is not finished
		}
		// Through the source, whatever kind of task this is. Skipping the ones sindri does not own
		// left an openspec parent open over a finished tree, and it is that leak — every site having
		// to remember which tasks it may finish — that put the same bug in four places.
		if err := e.finishAtSource(project, e.deps.ProjectRoot(project), parent, false); err != nil {
			fmt.Fprintf(os.Stderr, "hub: closing completed parent %s: %v\n", parent, err)
			return
		}
		e.settleWithTask(project, parent)
		_ = e.RefreshTask(project, parent)
	}
}

// gatedUnder is open work under a feature still AWAITING A VERDICT, at any depth — the COMPLETION
// question, since such a subtask is ABSENT from OpenSubtasks rather than reported by it. Pending
// only, never the claim rule: nothing clears a rejection, so blocking on one parks the holder.
func (e *Engine) gatedUnder(project, container string) ([]store.Task, error) {
	all, err := e.store.For(project).AllTasks()
	if err != nil {
		return nil, err
	}
	var out []store.Task
	for _, d := range api.Descendants(all, container) {
		if api.Open(d) && d.Approval == "pending" {
			out = append(out, d)
		}
	}
	return out, nil
}

// openIDs names these tasks, for a message that has to say WHICH work is holding a feature open.
func openIDs(tasks []store.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	return ids
}

// claimNextSubtask is claimNext's rule for a held feature's subtasks: the next open child, claimed
// FIRST so the reset that follows runs against work already held. None open means done or gated.
func (e *Engine) claimNextSubtask(ctx context.Context, project, agent, container string) (string, bool, error) {
	if e.clearArmed(project, agent) {
		return "", false, nil // clear fires at leaf boundary, no subtask claimed until after
	}
	child, advanced, err := e.advanceContainer(project, agent, container)
	if err != nil {
		return "", false, err
	}
	if !advanced {
		gated, err := e.gatedUnder(project, container)
		if err != nil {
			return "", false, err
		}
		if len(gated) > 0 {
			// false, not true: nothing was claimed, same as claimNext's own "nothing for you" — the
			// distinction assignPendingSubtask (task.go) depends on to tell a real hand-over from a
			// repeat of the same wait.
			return ReplyFeatureGated(container, openIDs(gated)), false, nil
		}
		return DirContainerDone(container), true, nil
	}
	// Claimed, whichever subtask it was — the "told about this while idle" memory ends here too
	// (-> claimNext's own reset).
	_ = e.store.For(project).SetLastNudge(agent, "")
	aim, ceiling := e.commentBudget(project)
	dir := DirContainerWorking(container, child.ID, aim, ceiling)
	fired, err := e.prepareAssignment(ctx, project, agent, api.TierOrDefault(child.Tier), dir)
	if err != nil {
		return "", false, err
	}
	if fired {
		return DirPreparing, true, nil
	}
	return dir, true, nil
}

// advanceContainer moves a held feature's agent onto its next open subtask: (subtask, true) when one
// was assigned, (zero, false) only when there is none. A failed assignment is an error rather than a
// quiet false, which read as "finished" while the submit gate, asking the same question, refused.
func (e *Engine) advanceContainer(project, agent, container string) (store.Task, bool, error) {
	ps := e.store.For(project)
	children, err := ps.OpenSubtasks(container)
	if err != nil {
		return store.Task{}, false, err
	}
	if len(children) == 0 {
		return store.Task{}, false, nil
	}
	child := children[0]
	if err := e.startSubtask(project, agent, container, child); err != nil {
		return store.Task{}, false, err
	}
	return child, true, nil
}

// startSubtask puts an agent on one subtask of the feature it holds.
func (e *Engine) startSubtask(project, agent, container string, child store.Task) error {
	ps := e.store.For(project)
	if err := e.SetStatus(project, child.ID, "in_progress"); err != nil {
		return err
	}
	_ = e.RefreshTask(project, child.ID)
	// Each subtask is a claim, so each grants the note budget afresh (-> store.GrantNotes).
	if err := ps.GrantNotes(agent, NotesPerClaim); err != nil {
		return err
	}
	if err := ps.SetState(store.AgentState{
		Agent: agent, Container: container, Branch: container, Task: child.ID, Phase: "working",
	}, store.ReasonClaimed, "claimed subtask "+child.ID+" under "+container); err != nil {
		return err
	}
	e.deps.Notify()
	return nil
}
