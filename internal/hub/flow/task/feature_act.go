// package: hub/flow/task / feature_act
// type:    logic (the subtask loop inside one feature branch)
// job:     working a task that has subtasks: claim the whole tree, hand out its open
// leaves one at a time on a single branch, checkpoint each, and close every
// parent the last of its children completes. The branch goes up as one PR
// (-> hub/flow/pr's CmdSubmit) when nothing open is left under the feature.
// limits:  assignment and closure within a held tree; the leaf-task path and the
// directive loop are task_act.go's, and the PR is hub/flow/pr's.
package task

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"github.com/flo-at/sindri/internal/hub/world/task"
	hubtask "github.com/flo-at/sindri/internal/hub/world/task"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// ClaimContainer assigns one package, starting its first open subtask — or, with none left,
// holding it so the agent finishes on the SAME branch (git.EnsureBranch), never a fresh one.
func (a *Act) ClaimContainer(project, worker string, c store.Task) (string, bool, error) {
	ps := a.Store.For(project)
	root := a.Deps.ProjectRoot(project)
	children, err := ps.OpenSubtasks(c.ID)
	if err != nil {
		return "", false, err
	}
	base, err := a.BaseBranch(root)
	if err != nil {
		return "", false, err
	}
	ag, ok, err := ps.GetAgent(worker)
	if err != nil || !ok {
		return "", false, fmt.Errorf("agent %s missing: %v", worker, err)
	}
	wt := filepath.Join(root, ag.Workspace)
	if err := git.EnsureBranch(wt, c.ID, base); err != nil {
		return "", false, err
	}
	if len(children) == 0 {
		if err := ps.SetHolding(worker, "", c.ID, c.ID,
			store.ReasonClaimed, "claimed container "+c.ID+" with nothing open under it"); err != nil {
			return "", false, err
		}
		_ = ps.Log(worker, "claim-container", c.ID+" "+c.Title+" (nothing left — finishing it)")
		return prompts.DirContainerDone(c.ID), true, nil
	}
	child := children[0]
	if err := a.StartSubtask(project, worker, c.ID, child); err != nil {
		return "", false, err
	}
	_ = ps.Log(worker, "claim-container", c.ID+" "+c.Title)
	return prompts.DirContainerClaimed(c.ID, c.Title, child.ID, child.Title), true, nil
}

// CmdCheckpoint commits the current subtask to the feature branch, closes that
// subtask, and advances to the next — staying working, never blocking for review.
func (a *Act) CmdCheckpoint(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := a.Store.For(c.Project)
	root := a.Deps.ProjectRoot(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Container == "" || !situation.Standing(st.Phase, "working") || st.Task == "" {
		fmt.Fprintln(out, prompts.ReplyNothingToCheckpoint)
		return 1, nil
	}
	// A rejection returns the work in phase "working", which is exactly the shape a checkpoint takes
	// for finished. austri checkpointed past a rejected pr-sd-a47b61, closing the task and freeing
	// itself for a second one it then had no room for.
	if pr, task, perr := ps.AwaitingPR(c.Agent); perr != nil {
		return 1, perr
	} else if pr != "" && task == st.Task {
		fmt.Fprintln(out, prompts.ReplyPRStillToLand(st.Task, pr))
		return 1, nil
	}
	ag, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, ag.Workspace)
	tk, _, _ := ps.GetTask(st.Task)
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		msg = tk.Title
	}
	if msg == "" {
		msg = "work on " + st.Task
	}
	msg = task.ConventionalCommit(tk.Type, st.Task, msg)
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
		if err := a.pr().FinishAtSource(c.Project, wt, st.Task, false); err != nil {
			return 1, err
		}
		// The step this path skipped by calling the source directly: without it AdvanceContainer
		// below re-picks the subtask just finished (-> recordEnded).
		if err := a.pr().RecordEnded(c.Project, st.Task); err != nil {
			return 1, err
		}
	}
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	if len(grew) == 0 {
		a.pr().SettleWithTask(c.Project, st.Task)
		_ = a.RefreshTask(c.Project, st.Task)
		a.closeCompletedAncestors(c.Project, st.Task, st.Container)
	}
	_ = ps.Log(c.Agent, "checkpoint", st.Task)
	done := st.Task
	// A checkpoint IS a leaf boundary, so an armed clear takes precedence over the next subtask:
	// the agent goes idle holding the feature, and the clear fires before anything else is served.
	if a.ClearArmedFor(c.Project, c.Agent) {
		_ = ps.SetHolding(c.Agent, "", st.Container, st.Container,
			store.ReasonFreed, "checkpointed "+done+" with a clear armed")
		a.announceHolding(c.Project, c.Agent)
		fmt.Fprintln(out, prompts.ReplyCheckpointedClearing(done, st.Container))
		return 0, nil
	}
	next, ok, err := a.AdvanceContainer(c.Project, c.Agent, st.Container)
	if err != nil {
		return 1, err
	}
	if ok {
		if len(grew) > 0 {
			fmt.Fprintln(out, prompts.ReplyCheckpointedParentOpen(done, grew, next.ID, next.Title))
			return 0, nil
		}
		fmt.Fprintln(out, prompts.ReplyCheckpointed(done, next.ID, next.Title))
		return 0, nil
	}
	gated, err := hubtask.GatedUnder(ps, st.Container)
	if err != nil {
		return 1, err
	}
	_ = ps.SetHolding(c.Agent, "", st.Container, st.Container,
		store.ReasonAdvanced, "checkpointed "+done+", nothing left to claim under "+st.Container)
	a.announceHolding(c.Project, c.Agent)
	if len(gated) > 0 {
		fmt.Fprintf(out, "Checkpointed %s. %s\n", done, prompts.ReplyFeatureGated(st.Container, hubtask.OpenIDs(gated)))
		return 0, nil
	}
	fmt.Fprintln(out, prompts.ReplyCheckpointedLast(done, st.Container))
	return 0, nil
}

// closeCompletedAncestors closes each parent above a just-closed task whose children are now all
// closed, stopping below stopAt (the feature itself, which its PR closes) — the only thing that
// marks an intermediate epic finished, or it stays open and is handed out as work that isn't there.
func (a *Act) closeCompletedAncestors(project, from, stopAt string) {
	ps := a.Store.For(project)
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
		if err := a.pr().FinishAtSource(project, a.Deps.ProjectRoot(project), parent, false); err != nil {
			fmt.Fprintf(os.Stderr, "hub: closing completed parent %s: %v\n", parent, err)
			return
		}
		a.pr().SettleWithTask(project, parent)
		_ = a.RefreshTask(project, parent)
	}
}

// AdvanceContainer moves a held feature's agent onto its next open subtask: (subtask, true) when one
// was Assigned, (zero, false) only when there is none. A failed assignment is an error rather than a
// quiet false, which read as "finished" while the submit gate, asking the same question, refused.
func (a *Act) AdvanceContainer(project, agent, container string) (store.Task, bool, error) {
	ps := a.Store.For(project)
	children, err := ps.OpenSubtasks(container)
	if err != nil {
		return store.Task{}, false, err
	}
	if len(children) == 0 {
		return store.Task{}, false, nil
	}
	child := children[0]
	if err := a.StartSubtask(project, agent, container, child); err != nil {
		return store.Task{}, false, err
	}
	return child, true, nil
}

// StartSubtask puts an agent on one subtask of the feature it holds.
func (a *Act) StartSubtask(project, agent, container string, child store.Task) error {
	ps := a.Store.For(project)
	if err := a.pr().SetStatus(project, child.ID, "in_progress"); err != nil {
		return err
	}
	_ = a.RefreshTask(project, child.ID)
	// Each subtask is a claim, so each grants the note budget afresh (-> store.GrantNotes).
	if err := ps.GrantNotes(agent, prompts.NotesPerClaim); err != nil {
		return err
	}
	if err := ps.SetHolding(agent, child.ID, container, container,
		store.ReasonClaimed, "claimed subtask "+child.ID+" under "+container); err != nil {
		return err
	}
	a.announceHolding(project, agent)
	return nil
}
