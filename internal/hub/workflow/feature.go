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
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// featureLanded reports a feature an agent should no longer be holding: closed at its source, or
// carried in by a PR that has merged. The PR half matters because a worker released only on status
// sat on a feature whose branch was already in the reference, being handed it again on every ask.
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

// claimContainer assigns one package to a worker, starting its first open subtask — or, with
// nothing left under it, holding it anyway so the agent finishes it on the SAME branch
// (git.EnsureBranch), never a fresh one. Which package is nextUp's (-> assign.go).
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
	// A task with work still open beneath it is not something a checkpoint can finish. Checked here
	// because this is what writes "closed": an epic handed out as a subtask was closed over four open
	// children of its own, and nothing downstream could tell that had happened.
	if open, oerr := ps.OpenChildIDs(st.Task); oerr != nil {
		return 1, oerr
	} else if len(open) > 0 {
		fmt.Fprintln(out, ReplyHasOpenChildren("checkpoint", st.Task, open))
		return 1, nil
	}
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	// Through the source, not owned_tasks: a subtask can be an openspec change or an issue, whose
	// status its own source keeps and a direct write would fail on.
	if err := e.finishAtSource(c.Project, root, st.Task, false); err != nil {
		return 1, err
	}
	_ = e.RefreshTask(c.Project, st.Task)
	e.closeCompletedAncestors(c.Project, st.Task, st.Container)
	_ = ps.Log(c.Agent, "checkpoint", st.Task)
	done := st.Task
	next, ok, err := e.advanceContainer(c.Project, c.Agent, st.Container)
	if err != nil {
		return 1, err
	}
	if ok {
		fmt.Fprintln(out, ReplyCheckpointed(done, next.ID, next.Title))
		return 0, nil
	}
	_ = ps.SetState(store.AgentState{Agent: c.Agent, Container: st.Container, Branch: st.Container, Phase: "idle"})
	e.deps.Notify()
	fmt.Fprintln(out, ReplyCheckpointedLast(done, st.Container))
	return 0, nil
}

// closeCompletedAncestors closes each parent above a just-closed task whose children are now all
// closed, stopping below stopAt (the feature itself, which its PR closes). A parent is done exactly
// when its children are, so this is the only thing that marks an intermediate epic finished — and
// without it one stays open forever, then reads as a leaf and is handed out as work that isn't there.
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
	if err := ps.SetState(store.AgentState{
		Agent: agent, Container: container, Branch: container, Task: child.ID, Phase: "working",
	}); err != nil {
		return err
	}
	e.deps.Notify()
	return nil
}
