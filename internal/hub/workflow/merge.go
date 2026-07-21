// package: hub/workflow / merge
// type:    logic (merge workflow)
// job:     the human-gated merge of an approved PR into its base — rebase-first,
//
//	conflict routing to the worker, and the transient "merging" status plus
//	startup reconciliation to "merge-failed" for a merge orphaned by a crash.
//
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

// ReconcileMergingPRs runs once at hub startup: any PR still in "merging" was being
// merged when the previous hub died, so its outcome is unknown (the base may or may
// not carry the merge). Move it to "merge-failed" — a visible state that asks for a
// human look — instead of leaving a silent, frozen "merging".
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
	// Persist an explicit in-flight status: the merge is triggered but its outcome
	// isn't known yet. This keeps the board honest (a moving "merging", not a frozen
	// "approved") AND makes it recoverable — if the hub dies mid-merge, startup
	// reconciles any leftover "merging" to "merge-failed" (see ReconcileMergingPRs),
	// because a half-applied merge needs a human look, not a silent retry.
	pr.Status = "merging"
	if err := ps.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	e.deps.Notify()
	// On a synchronous failure below, revert to approved so the PR stays retryable —
	// the returned error tells the human what to fix. (A conflict takes its own path
	// to "open"; a crash mid-merge is caught at startup, not here.)
	revert := func(err error) (store.PR, error) {
		pr.Status = "approved"
		_ = ps.PutPR(pr)
		e.deps.Notify()
		return store.PR{}, err
	}
	// Bring the branch up to base then merge — the git mechanics live in repo; here we
	// route the outcome. The rebase runs in the WORKER's worktree (it can't run git
	// itself); a conflict goes into its resolution loop, not a dead-end "resubmit".
	wt, workspace := "", ""
	if a, ok, _ := ps.GetAgent(pr.Agent); ok {
		workspace, wt = a.Workspace, filepath.Join(root, a.Workspace)
	}
	switch res := repo.MergeBranch(root, wt, pr.Branch, pr.Base); res.Status {
	case repo.MergeConflict:
		pr.Status, pr.Feedback = "open", "" // no longer mergeable; back to review after the worker resolves
		_ = ps.PutPR(pr)
		_ = ps.SetState(store.AgentState{Agent: pr.Agent, Task: pr.Task, Branch: pr.Branch, Phase: "resolving"})
		_ = ps.LogPR(pr.ID, "conflict", "rebase onto "+pr.Base+" conflicts: "+strings.Join(res.Files, ", "))
		_ = e.deps.InjectWhenReady(project, pr.Agent, MsgResolveNeeded(pr.Base, res.Files))
		e.deps.Notify()
		return store.PR{}, fmt.Errorf("%s conflicts with %s — sent to %s to resolve; it returns for review once clean", prID, pr.Base, pr.Agent)
	case repo.MergeRebaseErr:
		// The rebase runs in the AGENT's worktree, not your checkout — say so, or
		// "unstaged changes" sends you hunting in the wrong place. Usually the agent
		// left uncommitted edits there; it must commit or discard them.
		return revert(fmt.Errorf("can't rebase %s onto %s in %s's worktree (%s) — most often it has uncommitted changes (NOT your checkout); have the agent commit or discard them, e.g. `sindri agent tell %s \"commit or discard your /workspace changes, then say done\"`. git said: %w",
			pr.Branch, pr.Base, pr.Agent, workspace, pr.Agent, res.Err))
	case repo.MergeBlocked:
		return revert(fmt.Errorf("merge blocked: commit or stash your local changes to %s in the working checkout, then merge again (the PR is fine and stays approved)", FileList(res.Files)))
	case repo.MergeErr:
		return revert(res.Err)
	}
	pr.Status = "merged"
	if err := ps.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	// Interim contribution: land the work but KEEP the task open and put the worker
	// straight back on the SAME task. Its branch is fast-forwarded past the merge so it
	// keeps building on top; nothing is closed. (Container milestones take the branch
	// below; the two never overlap — contribute is hidden inside a container.)
	if pr.Kind == "interim" {
		if a, ok, _ := ps.GetAgent(pr.Agent); ok {
			_ = git.RebaseOnto(filepath.Join(root, a.Workspace), pr.Branch, pr.Base) // ff past the merge
		}
		_ = ps.SetState(store.AgentState{Agent: pr.Agent, Task: pr.Task, Branch: pr.Branch, Phase: "working"})
		_ = ps.Log(pr.Agent, "merged", prID+" (interim)")
		_ = ps.LogPR(prID, "merged", "interim contribution into "+pr.Base)
		_ = e.deps.InjectWhenReady(project, pr.Agent, MsgContributionMerged(prID, pr.Task))
		e.rebasePlanners(project, pr.Base) // any merge moves base → keep planners current
		e.deps.Notify()
		return pr, nil
	}
	// Milestone PR for a held container: land the work but KEEP the branch/
	if holder, _ := ps.GetState(pr.Agent); holder.Container != "" && holder.Container == pr.Branch {
		if a, ok, _ := ps.GetAgent(pr.Agent); ok {
			_ = git.RebaseOnto(filepath.Join(root, a.Workspace), pr.Branch, pr.Base) // ff past the merge
		}
		_ = ps.Log(pr.Agent, "merged", prID+" (milestone)")
		_ = ps.LogPR(prID, "merged", "milestone into "+pr.Base)
		e.resumeContainer(project, pr.Agent)
		_ = e.deps.InjectWhenReady(project, pr.Agent, MsgMilestoneMerged(prID))
		e.rebasePlanners(project, pr.Base)
		e.deps.Notify()
		return pr, nil
	}
	// The task's PR merged — notify every task source so it can run its own
	// consequence (td closes the task, github closes+comments the issue, openspec
	// no-ops). The workflow stays ignorant of which backend the task uses; each source
	// acts only on its own ids. Best-effort: it runs AFTER the local merge landed, so a
	// failure is a warning on the PR (may need a manual upstream follow-up), never a
	// merge failure.
	note := "merged via " + prID
	for _, src := range taskSources() {
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
