// package: hub/flow/pr / resolve_act
// type:    logic (worker-driven merge-conflict resolution)
// job:     the `resolve` verb — bring a worker's submitted branch up to its base,
// surfacing any conflict into the worker's workspace for it to edit while
// the hub drives all git; once clean, renew the PR for review.
// limits:  git mechanics live in adapter/git; the merge gate lives in gate_act.go.
package pr

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// reattach puts a detached worktree back on the agent's recorded branch: deletion frees a branch by
// detaching, and nothing put it back, so a rebase dead-ended on "detached HEAD" forever.
func (a *Act) reattach(ps *store.ProjectStore, agent, wt string) (string, error) {
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

// RebaseNotice fronts an agent's directive when a rebase has left its branch mid-flight, "" otherwise.
// A role text speaks about the next task or about the conversation, so a stuck branch went unsaid: one
// planner sat on 49 conflicted paths reading "the hub has nothing to add".
func (a *Act) RebaseNotice(project, name string) string {
	ps := a.Store.For(project)
	ag, ok, err := ps.GetAgent(name)
	if err != nil || !ok || ag.Workspace == "" {
		return ""
	}
	// Both states RebaseStep continues, for the reason CmdResolve pairs them: a stranded autostash
	// leaves the same unmergeable index, and git refuses every later checkout over it.
	wt := filepath.Join(a.Deps.ProjectRoot(project), ag.Workspace)
	if !git.RebaseStuck(wt) {
		return ""
	}
	st, _ := ps.GetState(name)
	verb := "rebase"
	if situation.Standing(st.Phase, "resolving") { // mid-merge for its own PR — resolve renews it
		verb = "resolve"
	}
	return prompts.DirBranchStuck(verb)
}

// CmdRebase is the agent-driven "align with the reference" verb, safe any time; WIP is autostashed.
// A conflict leaves the markers in /workspace and continues on the next `sindri rebase`.
func (a *Act) CmdRebase(c registry.Caller, _ []string, out io.Writer) (int, error) {
	ps := a.Store.For(c.Project)
	root := a.Deps.ProjectRoot(c.Project)
	ag, ok, err := ps.GetAgent(c.Agent)
	if err != nil || !ok {
		return 1, fmt.Errorf("agent %s missing: %v", c.Agent, err)
	}
	wt := filepath.Join(root, ag.Workspace)
	base, err := a.BaseBranch(root)
	if err != nil {
		return 1, err
	}
	branch := ""
	if !git.RebaseInProgress(wt) { // starting fresh — guard against rebasing base onto itself
		b, berr := git.CurrentBranch(wt)
		if berr != nil {
			if b, berr = a.reattach(ps, c.Agent, wt); berr != nil {
				return 1, berr
			}
		}
		if b == base {
			fmt.Fprintf(out, "You're on %s itself — nothing to rebase.\n", prompts.RefName)
			return 0, nil
		}
		branch = b
	}
	// Read what is arriving BEFORE the rebase: afterwards those commits are ancestors, and
	// indistinguishable from the agent's own. A continuation already reported them.
	var incoming []string
	if branch != "" {
		incoming, _ = git.LogRange(wt, branch, base, core.LogCap)
	}
	conflicts, done, err := git.RebaseStep(wt, branch, base)
	if err != nil {
		return 1, err
	}
	if !done {
		_ = ps.Log(c.Agent, "rebase", "conflicts: "+strings.Join(conflicts, ", "))
		// Asked AFTER the step: a stopped rebase is still in progress, while a clashing autostash
		// leaves only the index, and calling that "the rebase hit conflicts" misdirects the worker.
		if git.StashConflict(wt) {
			fmt.Fprintln(out, prompts.ReplyRebaseStashConflicts(conflicts))
		} else {
			fmt.Fprintln(out, prompts.ReplyRebaseConflicts(conflicts))
		}
		return 0, nil
	}
	_ = ps.Log(c.Agent, "rebase", "onto "+base)
	a.Deps.Notify()
	fmt.Fprintln(out, prompts.ReplyRebased(incoming))
	return 0, nil
}

// CmdResolve is the worker-driven mergeability loop: bring the submitted branch up to base, leaving
// any markers to edit and continuing next call. Once it applies cleanly the PR is renewed.
func (a *Act) CmdResolve(c registry.Caller, _ []string, out io.Writer) (int, error) {
	ps := a.Store.For(c.Project)
	root := a.Deps.ProjectRoot(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Branch == "" {
		fmt.Fprintln(out, "nothing to resolve — you have no submitted branch")
		return 1, nil
	}
	ag, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, ag.Workspace)
	base, err := a.BaseBranch(root)
	if err != nil {
		return 1, err
	}
	// A stranded autostash counts as mid-resolution too: its unmerged entries read as "uncommitted
	// work", and git refuses to commit an unmerged index, so that advice could not be followed.
	inProgress := git.RebaseStuck(wt)
	// A rebase needs a clean worktree, so uncommitted work is sent back to be recorded first —
	// except mid-resolution, where those edits ARE the resolution. Never touch them here.
	if !inProgress {
		changed, cerr := git.HasChanges(wt)
		if cerr != nil {
			return 1, cerr
		}
		if changed {
			fmt.Fprintln(out, prompts.ReplyResolveDirty(st.Phase, st.Container != ""))
			return 1, nil
		}
	}
	// A milestone PR is keyed by the CONTAINER, not the subtask st.Task holds while resolving
	// (-> merge_act.go's own reset step keeps that distinct for ResumeContainer's sake).
	prKey := st.Task
	if st.Container != "" {
		prKey = st.Container
	}
	prID := "pr-" + prKey
	// Two readings of one question, because a conflict reaches an author by two routes: recorded
	// against the pull request by whoever hit it (-> cond.MergeConflicted), or shown by the machine
	// having stood the author in its resolving state, which a reapply clash does with no pull request
	// involved at all. Both are somebody else's account — neither is this verb's own copy of it.
	standing := situation.Standing(st.Phase, "resolving") || ConflictStanding(ps, prID)
	conflicts, done, err := git.RebaseStep(wt, st.Branch, base)
	if err != nil {
		return 1, err // internal git failure — AgentExec sanitizes it for the agent
	}
	if !done {
		a.RecordConflict(ps, prID, base, conflicts)
		_ = ps.Log(c.Agent, "resolve", "conflicts: "+strings.Join(conflicts, ", "))
		a.announceVerdict(c.Project)
		fmt.Fprintln(out, prompts.ReplyResolveConflicts(base, conflicts))
		return 0, nil
	}
	// Only a completed CONFLICT resolution changed the branch and needs re-review; a proactive
	// check on an already-current one renews nothing.
	if standing {
		pr, ok, _ := ps.GetPR(prID)
		// A MERGED pr is the signal, not its absence: a standing branch's own uncommitted work
		// failed to reapply after its PR already merged (-> merge_act.go), so resolving it
		// resumes exactly as a clean reset would have — never renew or re-review a merged PR.
		if ok && pr.Status == "merged" {
			onFeature := st.Container != "" && st.Container == pr.Branch
			if _, ferr := a.finishPartialMerge(c.Project, pr, onFeature); ferr != nil {
				return 1, ferr
			}
			fmt.Fprintln(out, prompts.ReplyReapplyResolved())
			return 0, nil
		}
		reply := prompts.ReplyResolvedClean(base)
		if ok {
			pr.Status, pr.Feedback = "open", ""
			_ = ps.PutPR(pr)
			_ = ps.LogPR(pr.ID, "renewed", "rebased clean onto "+base)
			if pr.Kind == "interim" {
				reply = prompts.ReplyContributionClean(base) // interim PRs are user-gated — no reviewer
			} else {
				_ = a.RequestReview(c.Project, pr.ID, "") // one review path; the hub preps the terrain
			}
		}
		_ = ps.Log(c.Agent, "resolve", "clean onto "+base)
		fmt.Fprintln(out, reply)
		a.announceVerdict(c.Project)
		return 0, nil
	}
	fmt.Fprintln(out, prompts.ReplyAlreadyCurrent(base))
	return 0, nil
}
