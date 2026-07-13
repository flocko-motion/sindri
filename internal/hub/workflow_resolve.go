// package: hub / workflow_resolve
// type:    logic (worker-driven merge-conflict resolution)
// job:     the `resolve` verb — bring a worker's submitted branch up to its base,
//
//	surfacing any conflict into the worker's workspace for it to edit while
//	the hub drives all git; once clean, renew the PR for review.
//
// limits:  git mechanics live in adapter/git; the merge gate lives in workflow_pr.
package hub

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/workflow"
	"io"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// cmdRebase is the agent-driven "align with the reference" verb: it rebases the
// agent's own worktree branch onto the current reference branch (whatever the main
// checkout has checked out), any time the agent likes — there's no harm, it just
// keeps the agent current. Uncommitted WIP is autostashed around the rebase. On a
// conflict it leaves the markers in the agent's /workspace and tells it which files
// to fix, then continues on the next `sindri rebase`; once clean it reports aligned.
// All git runs host-side (the agent has none).
func (h *Hub) cmdRebase(c registry.Caller, _ []string, out io.Writer) (int, error) {
	ps := h.store.For(c.Project)
	root := h.projectRoot(c.Project)
	a, ok, err := ps.GetAgent(c.Agent)
	if err != nil || !ok {
		return 1, fmt.Errorf("agent %s missing: %v", c.Agent, err)
	}
	wt := filepath.Join(root, a.Workspace)
	base, err := h.baseBranch(root)
	if err != nil {
		return 1, err
	}
	var conflicts []string
	var done bool
	if git.RebaseInProgress(wt) { // a previous rebase is mid-conflict — advance it
		conflicts, done, err = git.RebaseContinue(wt)
	} else {
		branch, berr := git.CurrentBranch(wt)
		if berr != nil {
			return 1, berr
		}
		if branch == base {
			fmt.Fprintf(out, "You're on %s (the reference branch itself) — nothing to rebase.\n", base)
			return 0, nil
		}
		conflicts, done, err = git.RebaseStart(wt, branch, base)
	}
	if err != nil {
		return 1, err
	}
	if !done {
		_ = ps.Log(c.Agent, "rebase", "conflicts: "+strings.Join(conflicts, ", "))
		fmt.Fprintln(out, workflow.ReplyRebaseConflicts(base, conflicts))
		return 0, nil
	}
	_ = ps.Log(c.Agent, "rebase", "onto "+base)
	h.notify()
	fmt.Fprintln(out, workflow.ReplyRebased(base))
	return 0, nil
}

// cmdResolve is the worker-driven mergeability loop: it brings the worker's
// submitted branch up to its base, and when that conflicts it leaves the conflict
// markers in the worker's workspace for it to edit, then continues on the next
// call. The worker may run it as often as it likes. All git runs host-side (the
// worker has none); once the branch applies cleanly the PR is renewed for review.
func (h *Hub) cmdResolve(c registry.Caller, _ []string, out io.Writer) (int, error) {
	ps := h.store.For(c.Project)
	root := h.projectRoot(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Branch == "" {
		fmt.Fprintln(out, "nothing to resolve — you have no submitted branch")
		return 1, nil
	}
	a, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, a.Workspace)
	base, err := h.baseBranch(root)
	if err != nil {
		return 1, err
	}
	inProgress := git.RebaseInProgress(wt)
	// A rebase needs a clean worktree. If the worker has uncommitted work (and isn't
	// mid-resolution, where the edits ARE the resolution), tell it to commit first —
	// don't touch its changes.
	if !inProgress {
		changed, cerr := git.HasChanges(wt)
		if cerr != nil {
			return 1, cerr
		}
		if changed {
			fmt.Fprintln(out, "you have uncommitted changes — run `sindri submit` (or commit) first, then check with `sindri resolve`")
			return 1, nil
		}
	}
	var conflicts []string
	var done bool
	if inProgress { // a resolution already underway — advance it
		conflicts, done, err = git.RebaseContinue(wt)
	} else {
		conflicts, done, err = git.RebaseStart(wt, st.Branch, base)
	}
	if err != nil {
		return 1, err // internal git failure — AgentExec sanitizes it for the agent
	}
	if !done {
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "resolving"})
		_ = ps.Log(c.Agent, "resolve", "conflicts: "+strings.Join(conflicts, ", "))
		fmt.Fprintln(out, workflow.ReplyResolveConflicts(base, conflicts))
		return 0, nil
	}
	// Clean. Only a completed *conflict* resolution changed the branch and needs the
	// PR renewed for re-review; a proactive check on an already-current branch leaves
	// the phase (working/submitted) untouched.
	if st.Phase == "resolving" {
		if pr, ok, _ := ps.GetPR("pr-" + st.Task); ok {
			pr.Status, pr.Feedback = "open", ""
			_ = ps.PutPR(pr)
			_ = ps.LogPR(pr.ID, "renewed", "rebased clean onto "+base)
			_ = h.RequestReview(c.Project, pr.ID, "") // one review path; the hub preps the terrain
		}
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "submitted"})
		fmt.Fprintln(out, workflow.ReplyResolvedClean(base))
		h.notify()
		return 0, nil
	}
	fmt.Fprintln(out, workflow.ReplyAlreadyCurrent(base))
	return 0, nil
}
