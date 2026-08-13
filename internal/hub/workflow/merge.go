// package: hub/workflow / merge
// type:    logic (merge workflow)
// job:     the human-gated merge of an approved PR into its base — rebase-first,
// conflict routing to the worker, and the transient "merging" status plus
// startup reconciliation to "merge-failed" for a merge orphaned by a crash.
// limits:  merge only; review/submit live in workflow_pr.go (same hub package).
package workflow

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
)

// ReconcileMergingPRs runs at hub startup: a PR still "merging" was mid-merge when the last hub
// died, so nobody knows whether base carries it. "merge-failed" asks for a look; "merging" wouldn't.
func (e *Engine) ReconcileMergingPRs() {
	prs, err := e.store.AllPRs()
	if err != nil {
		log.Printf("hub: reconcile merging PRs: %v", err)
		return
	}
	for _, pr := range prs {
		if pr.Status != "merging" {
			continue
		}
		pr.Status = "merge-failed"
		ps := e.store.For(pr.Project)
		if err := ps.PutPR(pr); err != nil {
			log.Printf("hub: mark %s merge-failed: %v", pr.ID, err)
			continue
		}
		_ = ps.LogPR(pr.ID, "merge-failed", "hub restarted mid-merge — outcome unknown; inspect the base branch, then re-approve to retry")
		log.Printf("hub: %s was mid-merge at restart → merge-failed (inspect %s)", pr.ID, pr.Base)
	}
}

// Merge merges a project's approved PR into the base branch (host/human-only — the
// single hard gate), closes the task, frees the worker, and notifies it.
func (e *Engine) Merge(project, prID string) (store.PR, error) {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return store.PR{}, err
	}
	if !ok {
		return store.PR{}, fmt.Errorf("no such PR %q", prID)
	}
	if pr.Status != "approved" {
		return store.PR{}, fmt.Errorf("%s is %s — only an approved PR may be merged", prID, pr.Status)
	}
	// An explicit in-flight status keeps the board honest and makes a crash recoverable: startup
	// reconciles a leftover "merging" to "merge-failed", since half a merge needs a human.
	pr.Status = "merging"
	if err := ps.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	e.deps.Notify()
	// On a synchronous failure below, revert to approved so the PR stays retryable and the error
	// says what to fix. A conflict goes its own way to "open"; a crash is caught at startup.
	revert := func(err error) (store.PR, error) {
		pr.Status = "approved"
		_ = ps.PutPR(pr)
		e.deps.Notify()
		return store.PR{}, err
	}
	// Mechanics live in repo; here we route the outcome. The rebase runs in the WORKER's worktree,
	// and a conflict goes into its resolution loop rather than a dead-end "resubmit".
	wt, workspace := "", ""
	if a, ok, _ := ps.GetAgent(pr.Agent); ok {
		workspace, wt = a.Workspace, filepath.Join(root, a.Workspace)
	}
	tk, _, _ := ps.GetTask(pr.Task)
	desc := tk.Title
	if desc == "" {
		desc = pr.Task
	}
	mergeMsg := conventionalCommit(tk.Type, pr.Task, desc)
	switch res := repo.MergeBranch(root, wt, pr.Branch, pr.Base, mergeMsg); res.Status {
	case repo.MergeConflict:
		pr.Status, pr.Feedback = "open", "" // no longer mergeable; back to review after the worker resolves
		_ = ps.PutPR(pr)
		_ = ps.SetState(store.AgentState{Agent: pr.Agent, Task: pr.Task, Branch: pr.Branch, Phase: "resolving"})
		_ = ps.LogPR(pr.ID, "conflict", "rebase onto "+pr.Base+" conflicts: "+strings.Join(res.Files, ", "))
		_ = e.deps.InjectWhenReady(project, pr.Agent, MsgResolveNeeded(pr.Base, res.Files))
		e.deps.Notify()
		return store.PR{}, fmt.Errorf("%s conflicts with %s — sent to %s to resolve; it returns for review once clean", prID, pr.Base, pr.Agent)
	case repo.MergeRebaseErr:
		// Name the worktree: it is the AGENT's, and "unstaged changes" otherwise sends you
		// hunting through your own checkout for edits the agent left in its.
		return revert(fmt.Errorf("can't rebase %s onto %s in %s's worktree (%s) — most often it has uncommitted changes (NOT your checkout); have the agent commit or discard them, e.g. `sindri agent tell %s \"commit or discard your /workspace changes, then say done\"`. git said: %w",
			pr.Branch, pr.Base, pr.Agent, workspace, pr.Agent, res.Err))
	case repo.MergeBlocked:
		// "commit or stash" alone dead-ends an untracked collision: you cannot stash an untracked file.
		return revert(fmt.Errorf("merge blocked by your working checkout: %s. Commit or stash them (or move/remove them, if untracked), then merge again — the PR is fine and stays approved", FileList(res.Files)))
	case repo.MergeErr:
		return revert(res.Err)
	}
	pr.Status = "merged"
	if err := ps.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	// Any review still out on it is moot, and its reviewer is released and told so — the same thing
	// ScrapPR does. Left open, the reviewer kept being handed a merged PR, read an empty diff, and
	// its rejection overwrote the merge in the record.
	e.releaseReviewers(project, prID, "overtaken: merged before a verdict")
	// A landing that does not finish the work: the task stays open and its author stays on it, with
	// the branch fast-forwarded past the merge. Two shapes arrive here — a mid-task contribution, and
	// a milestone on a held feature that still has subtasks. A feature with none left IS finished by
	// this merge and takes the ordinary path below; keeping it here left a worker holding a feature
	// that had already landed, with its task still reading open.
	holder, _ := ps.GetState(pr.Agent)
	onFeature := holder.Container != "" && holder.Container == pr.Branch
	partial := pr.Kind == "interim"
	if onFeature {
		open, oerr := ps.OpenSubtasks(holder.Container)
		if oerr != nil {
			return store.PR{}, oerr
		}
		partial = len(open) > 0
	}
	if partial {
		if a, ok, _ := ps.GetAgent(pr.Agent); ok {
			_ = git.RebaseOnto(filepath.Join(root, a.Workspace), pr.Branch, pr.Base) // ff past the merge
		}
		if onFeature {
			_ = ps.Log(pr.Agent, "merged", prID+" (milestone)")
			_ = ps.LogPR(prID, "merged", "milestone into "+pr.Base)
			e.resumeContainer(project, pr.Agent)
			_ = e.deps.InjectWhenReady(project, pr.Agent, MsgMilestoneMerged(prID))
		} else {
			_ = ps.SetState(store.AgentState{Agent: pr.Agent, Task: pr.Task, Branch: pr.Branch, Phase: "working"})
			_ = ps.Log(pr.Agent, "merged", prID+" (interim)")
			_ = ps.LogPR(prID, "merged", "interim contribution into "+pr.Base)
			_ = e.deps.InjectWhenReady(project, pr.Agent, MsgContributionMerged(prID, pr.Task))
		}
		e.rebasePlanners(project, pr.Base) // any merge moves base → keep planners current
		e.deps.Notify()
		return pr, nil
	}
	// Tell every task source, so each runs its own consequence on its own ids and the workflow
	// need not know the backend. After the local merge, so a failure warns rather than fails it.
	note := "merged via " + prID
	for _, src := range e.taskSources(project) {
		if err := src.OnMerged(root, pr.Task, note); err != nil {
			log.Printf("hub: %s merged locally but a task-source close failed: %v", prID, err)
			_ = ps.LogPR(prID, "warning", "merged locally, but closing the task upstream failed (may need a manual follow-up): "+err.Error())
		}
	}
	e.refreshCachedTask(project, pr.Task) // reflect the now-closed task in the cache
	rest := "idle"
	if a, ok, _ := ps.GetAgent(pr.Agent); ok {
		rest = restPhase(a.Role)
	}
	_ = ps.SetState(store.AgentState{Agent: pr.Agent, Phase: rest})
	_ = ps.Log(pr.Agent, "merged", prID)
	_ = ps.LogPR(prID, "merged", "into "+pr.Base)
	_ = e.deps.InjectWhenReady(project, pr.Agent, MsgMerged(prID))
	e.rebasePlanners(project, pr.Base) // any merge moves base → keep planners current
	e.deps.Notify()
	return pr, nil
}
