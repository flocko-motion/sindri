// package: hub/workflow / resolve
// type:    logic (worker-driven merge-conflict resolution)
// job:     the `resolve` verb — bring a worker's submitted branch up to its base,
// surfacing any conflict into the worker's workspace for it to edit while
// the hub drives all git; once clean, renew the PR for review.
// limits:  git mechanics live in adapter/git; the merge gate lives in workflow_pr.
package workflow

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
)

// reattach puts a detached worktree back on the agent's recorded branch: deletion frees a branch by
// detaching, and nothing put it back, so a rebase dead-ended on "detached HEAD" forever.
func (e *Engine) reattach(ps *store.ProjectStore, agent, wt string) (string, error) {
	st, _ := ps.GetState(agent)
	if st.Branch == "" {
		return "", fmt.Errorf("%s is on a detached HEAD and the hub has no branch recorded for it", agent)
	}
	rescue, err := git.AttachBranch(wt, st.Branch)
	if err != nil {
		return "", err
	}
	note := "reattached to " + st.Branch
	if rescue != "" {
		// Loud: the branch had diverged, so the commits HEAD carried live under another name now.
		note += " — commits from the detached HEAD saved on " + rescue
		fmt.Fprintf(os.Stderr, "hub: %s %s\n", agent, note)
	}
	_ = ps.Log(agent, "rebase", note)
	return st.Branch, nil
}

// CmdRebase is the agent-driven "align with the reference" verb, safe any time; WIP is autostashed.
// A conflict leaves the markers in /workspace and continues on the next `sindri rebase`.
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
			if b, berr = e.reattach(ps, c.Agent, wt); berr != nil {
				return 1, berr
			}
		}
		if b == base {
			fmt.Fprintf(out, "You're on %s itself — nothing to rebase.\n", refName)
			return 0, nil
		}
		branch = b
	}
	// Read what is arriving BEFORE the rebase: afterwards those commits are ancestors, and
	// indistinguishable from the agent's own. A continuation already reported them.
	var incoming []string
	if branch != "" {
		incoming, _ = git.LogRange(wt, branch, base, logCap)
	}
	conflicts, done, err := repo.RebaseStep(wt, branch, base)
	if err != nil {
		return 1, err
	}
	if !done {
		_ = ps.Log(c.Agent, "rebase", "conflicts: "+strings.Join(conflicts, ", "))
		// Asked AFTER the step: a stopped rebase is still in progress, while a clashing autostash
		// leaves only the index, and calling that "the rebase hit conflicts" misdirects the worker.
		if git.StashConflict(wt) {
			fmt.Fprintln(out, ReplyRebaseStashConflicts(conflicts))
		} else {
			fmt.Fprintln(out, ReplyRebaseConflicts(conflicts))
		}
		return 0, nil
	}
	_ = ps.Log(c.Agent, "rebase", "onto "+base)
	e.deps.Notify()
	fmt.Fprintln(out, ReplyRebased(incoming))
	return 0, nil
}

// CmdResolve is the worker-driven mergeability loop: bring the submitted branch up to base, leaving
// any markers to edit and continuing next call. Once it applies cleanly the PR is renewed.
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
	// A stranded autostash counts as mid-resolution too: its unmerged entries read as "uncommitted
	// work", and git refuses to commit an unmerged index, so that advice could not be followed.
	inProgress := git.RebaseInProgress(wt) || git.StashConflict(wt)
	// A rebase needs a clean worktree, so uncommitted work is sent back to be recorded first —
	// except mid-resolution, where those edits ARE the resolution. Never touch them here.
	if !inProgress {
		changed, cerr := git.HasChanges(wt)
		if cerr != nil {
			return 1, cerr
		}
		if changed {
			fmt.Fprintln(out, ReplyResolveDirty(st.Phase, st.Container != ""))
			return 1, nil
		}
	}
	conflicts, done, err := repo.RebaseStep(wt, st.Branch, base)
	if err != nil {
		return 1, err // internal git failure — AgentExec sanitizes it for the agent
	}
	if !done {
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "resolving"},
			store.ReasonAdvanced, "rebase conflicts: "+strings.Join(conflicts, ", "))
		_ = ps.Log(c.Agent, "resolve", "conflicts: "+strings.Join(conflicts, ", "))
		fmt.Fprintln(out, ReplyResolveConflicts(base, conflicts))
		return 0, nil
	}
	// Only a completed CONFLICT resolution changed the branch and needs re-review; a proactive
	// check on an already-current one leaves the phase alone.
	if st.Phase == "resolving" {
		// A milestone PR is keyed by the CONTAINER, not the subtask st.Task holds while resolving
		// (-> workflow/merge.go's own reset step keeps that distinct for resumeContainer's sake).
		prKey := st.Task
		if st.Container != "" {
			prKey = st.Container
		}
		pr, ok, _ := ps.GetPR("pr-" + prKey)
		// A MERGED pr is the signal, not its absence: a standing branch's own uncommitted work
		// failed to reapply after its PR already merged (-> workflow/merge.go), so resolving it
		// resumes exactly as a clean reset would have — never renew or re-review a merged PR.
		if ok && pr.Status == "merged" {
			onFeature := st.Container != "" && st.Container == pr.Branch
			if _, ferr := e.finishPartialMerge(c.Project, pr, onFeature); ferr != nil {
				return 1, ferr
			}
			fmt.Fprintln(out, ReplyReapplyResolved())
			return 0, nil
		}
		reply := ReplyResolvedClean(base)
		if ok {
			pr.Status, pr.Feedback = "open", ""
			_ = ps.PutPR(pr)
			_ = ps.LogPR(pr.ID, "renewed", "rebased clean onto "+base)
			if pr.Kind == "interim" {
				reply = ReplyContributionClean(base) // interim PRs are user-gated — no reviewer
			} else {
				_ = e.RequestReview(c.Project, pr.ID, "") // one review path; the hub preps the terrain
			}
		}
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "submitted"},
			store.ReasonAdvanced, "resolved clean onto "+base)
		fmt.Fprintln(out, reply)
		e.deps.Notify()
		return 0, nil
	}
	fmt.Fprintln(out, ReplyAlreadyCurrent(base))
	return 0, nil
}
