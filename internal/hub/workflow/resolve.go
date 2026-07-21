// package: hub/workflow / resolve
// type:    logic (worker-driven merge-conflict resolution)
// job:     the `resolve` verb — bring a worker's submitted branch up to its base,
//
//	surfacing any conflict into the worker's workspace for it to edit while
//	the hub drives all git; once clean, renew the PR for review.
//
// limits:  git mechanics live in adapter/git; the merge gate lives in workflow_pr.
package workflow

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
)

// CmdRebase is the agent-driven "align with the reference" verb: it rebases the
// agent's own worktree branch onto the current reference branch (whatever the main
// checkout has checked out), any time the agent likes — there's no harm, it just
// keeps the agent current. Uncommitted WIP is autostashed around the rebase. On a
// conflict it leaves the markers in the agent's /workspace and tells it which files
// to fix, then continues on the next `sindri rebase`; once clean it reports aligned.
// All git runs host-side (the agent has none).
func (e *Engine) CmdRebase(c registry.Caller, _ []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	root := e.deps.ProjectRoot(c.Project)
	a, ok, err := ps.GetAgent(c.Agent)
	if err != nil || !ok {
		return 1, fmt.Errorf("agent %s missing: %v", c.Agent, err)
	}
	wt := filepath.Join(root, a.Workspace)
	base, err := e.baseBranch(root)
	if err != nil {
		return 1, err
	}
	branch := ""
	if !git.RebaseInProgress(wt) { // starting fresh — guard against rebasing base onto itself
		b, berr := git.CurrentBranch(wt)
		if berr != nil {
			return 1, berr
		}
		if b == base {
			fmt.Fprintf(out, "You're on %s (the reference branch itself) — nothing to rebase.\n", base)
			return 0, nil
		}
		branch = b
	}
	conflicts, done, err := repo.RebaseStep(wt, branch, base)
	if err != nil {
		return 1, err
	}
	if !done {
		_ = ps.Log(c.Agent, "rebase", "conflicts: "+strings.Join(conflicts, ", "))
		fmt.Fprintln(out, ReplyRebaseConflicts(base, conflicts))
		return 0, nil
	}
	_ = ps.Log(c.Agent, "rebase", "onto "+base)
	e.deps.Notify()
	fmt.Fprintln(out, ReplyRebased(base))
	return 0, nil
}

// CmdResolve is the worker-driven mergeability loop: it brings the worker's
// submitted branch up to its base, and when that conflicts it leaves the conflict
// markers in the worker's workspace for it to edit, then continues on the next
// call. The worker may run it as often as it likes. All git runs host-side (the
// worker has none); once the branch applies cleanly the PR is renewed for review.
func (e *Engine) CmdResolve(c registry.Caller, _ []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	root := e.deps.ProjectRoot(c.Project)
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
	base, err := e.baseBranch(root)
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
	conflicts, done, err := repo.RebaseStep(wt, st.Branch, base)
	if err != nil {
		return 1, err // internal git failure — AgentExec sanitizes it for the agent
	}
	if !done {
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "resolving"})
		_ = ps.Log(c.Agent, "resolve", "conflicts: "+strings.Join(conflicts, ", "))
		fmt.Fprintln(out, ReplyResolveConflicts(base, conflicts))
		return 0, nil
	}
	// Clean. Only a completed *conflict* resolution changed the branch and needs the
	// PR renewed for re-review; a proactive check on an already-current branch leaves
	// the phase (working/submitted) untouched.
	if st.Phase == "resolving" {
		reply := ReplyResolvedClean(base)
		if pr, ok, _ := ps.GetPR("pr-" + st.Task); ok {
			pr.Status, pr.Feedback = "open", ""
			_ = ps.PutPR(pr)
			_ = ps.LogPR(pr.ID, "renewed", "rebased clean onto "+base)
			if pr.Kind == "interim" {
				reply = ReplyContributionClean(base) // interim PRs are user-gated — no reviewer
			} else {
				_ = e.RequestReview(c.Project, pr.ID, "") // one review path; the hub preps the terrain
			}
		}
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "submitted"})
		fmt.Fprintln(out, reply)
		e.deps.Notify()
		return 0, nil
	}
	fmt.Fprintln(out, ReplyAlreadyCurrent(base))
	return 0, nil
}
