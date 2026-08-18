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

// featureLanded reports a feature an agent should no longer hold: closed at its source, or carried
// in by a merged PR — the half that matters, since status alone left a landed branch handed out again.
func featureLanded(ps *store.ProjectStore, t store.Task) bool {
	if t.Status == "closed" || t.Status == "approved" || t.Status == "merged" {
		return true
	}
	prs, err := ps.PRs()
	if err != nil {
		return false
	}
	for _, p := range prs {
		if p.Task == t.ID && p.Status == "merged" {
			return true
		}
	}
	return false
}

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
		if err := ps.SetState(store.AgentState{Agent: worker, Container: c.ID, Branch: c.ID, Phase: "idle"}); err != nil {
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
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	if len(grew) == 0 {
		// Through the source, not owned_tasks: a subtask can be an openspec change or an issue, whose
		// status its own source keeps and a direct write would fail on.
		if err := e.finishAtSource(c.Project, root, st.Task, false); err != nil {
			return 1, err
		}
		_ = e.RefreshTask(c.Project, st.Task)
		e.closeCompletedAncestors(c.Project, st.Task, st.Container)
	}
	_ = ps.Log(c.Agent, "checkpoint", st.Task)
	done := st.Task
	// A checkpoint IS a leaf boundary, so an armed clear takes precedence over the next subtask:
	// the agent goes idle holding the feature, and the clear fires before anything else is served.
	if e.clearArmed(c.Project, c.Agent) {
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Container: st.Container, Branch: st.Container, Phase: "idle"})
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
	_ = ps.SetState(store.AgentState{Agent: c.Agent, Container: st.Container, Branch: st.Container, Phase: "idle"})
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

// claimNextSubtask is claimNext's one-pass rule for a held feature's own subtasks; with none open,
// containerNext decides finished vs. still gated. A model change or compaction ends the pass here
// too, same as claimNext's — mail and containerNext only run once neither fires.
func (e *Engine) claimNextSubtask(project, agent, container string) (string, bool, error) {
	if e.clearArmed(project, agent) {
		return "", false, nil // about to land (fired by the caller): a subtask claimed now would be cut in half by it
	}
	children, err := e.store.For(project).OpenSubtasks(container)
	if err != nil {
		return "", false, err
	}
	if len(children) > 0 {
		tier := api.TierOrDefault(children[0].Tier)
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
	if d, has, err := e.pendingMail(project, agent); err != nil {
		return "", false, err
	} else if has {
		return d, true, nil
	}
	return e.containerNext(project, agent, container)
}

// containerNext is the held feature's next step: the subtask just assigned, or the finished feature
// to put up. Not ready while work awaits a verdict, so the worker waits (woken by its Notify).
func (e *Engine) containerNext(project, agent, container string) (string, bool, error) {
	next, ok, err := e.advanceContainer(project, agent, container)
	if err != nil {
		return "", false, err
	}
	if ok {
		aim, ceiling := e.commentBudget(project)
		return DirContainerWorking(container, next.ID, aim, ceiling), true, nil
	}
	gated, err := e.gatedUnder(project, container)
	if err != nil {
		return "", false, err
	}
	if len(gated) > 0 {
		return "", false, nil
	}
	return DirContainerDone(container), true, nil
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
	}); err != nil {
		return err
	}
	e.deps.Notify()
	return nil
}
