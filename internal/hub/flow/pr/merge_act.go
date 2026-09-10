// package: hub/flow/pr / merge_act
// type:    logic (merge workflow)
// job:     the human-gated merge of an approved PR into its base — rebase-first,
// conflict routing to the worker, and the transient "merging" status plus
// startup reconciliation to "merge-failed" for a merge orphaned by a crash.
// limits:  merge only; review and submit live beside this (review_act.go, pr_act.go).
package pr

import (
	"fmt"
	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/task"
	"log"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/hub/flow/topic"
	"github.com/flo-at/sindri/internal/hub/store"
)

// ReconcileMergingPRs settles anything a previous hub died holding — the map's own Orphaned exit
// now, asked at boot rather than on the next beat (-> hub/flow/pr).
func (a *Act) ReconcileMergingPRs() {
	projects, err := a.Store.Projects()
	if err != nil {
		log.Printf("hub: reconcile merging PRs: %v", err)
		return
	}
	for _, p := range projects {
		a.Flow.LookPRs(p.Tag)
	}
}

// Merge merges a project's approved PR into the base branch (host/human-only — the
// single hard gate), closes the task, frees the worker, and notifies it.
func (a *Act) Merge(project, prID string) (store.PR, error) {
	ps := a.Store.For(project)
	root := a.Deps.ProjectRoot(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return store.PR{}, err
	}
	if !ok {
		return store.PR{}, fmt.Errorf("%w %q", core.ErrNoSuchPR, prID)
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
	a.Deps.Notify()
	// On a synchronous failure below, revert to approved so the PR stays retryable and the error
	// says what to fix. A conflict goes its own way to "open"; a crash is caught at startup.
	revert := func(err error) (store.PR, error) {
		pr.Status = "approved"
		_ = ps.PutPR(pr)
		a.Deps.Notify()
		return store.PR{}, err
	}
	// Mechanics live in repo; here we route the outcome. The rebase runs in the WORKER's worktree,
	// and a conflict goes into its resolution loop rather than a dead-end "resubmit".
	wt, workspace := "", ""
	if ag, ok, _ := ps.GetAgent(pr.Agent); ok {
		workspace, wt = ag.Workspace, filepath.Join(root, ag.Workspace)
	}
	tk, _, _ := ps.GetTask(pr.Task)
	desc := tk.Title
	if desc == "" {
		desc = pr.Task
	}
	mergeMsg := task.ConventionalCommit(tk.Type, pr.Task, desc)
	switch res := git.MergeBranch(root, wt, pr.Branch, pr.Base, mergeMsg); res.Status {
	case git.MergeConflict:
		pr.Status, pr.Feedback = "open", "" // no longer mergeable; back to review after the worker resolves
		_ = ps.PutPR(pr)
		// Recorded on the PR, not written onto its author: a conflict is a fact about this merge
		// intent, and what it means for whoever filed it is their map's (-> cond.MergeConflicted).
		_ = ps.LogPR(pr.ID, "conflict", "rebase onto "+pr.Base+" conflicts: "+strings.Join(res.Files, ", "))
		a.Flow.WakeProject(project, topic.PRVerdict)
		_ = a.Harness.Say(project, pr.Agent, prompts.MsgResolveNeeded(pr.Base, res.Files), mail.MailAndPush)
		a.Deps.Notify()
		return store.PR{}, fmt.Errorf("%s conflicts with %s — sent to %s to resolve; it returns for review once clean", prID, pr.Base, pr.Agent)
	case git.MergeRebaseErr:
		// Name the worktree: it is the AGENT's, and "unstaged changes" otherwise sends you
		// hunting through your own checkout for edits the agent left in its.
		return revert(fmt.Errorf("can't rebase %s onto %s in %s's worktree (%s) — most often it has uncommitted changes (NOT your checkout); have the agent commit or discard them, ag.g. `sindri agent tell %s \"commit or discard your /workspace changes, then say done\"`. git said: %w",
			pr.Branch, pr.Base, pr.Agent, workspace, pr.Agent, res.Err))
	case git.MergeBlocked:
		// "commit or stash" alone dead-ends an untracked collision: you cannot stash an untracked file.
		return revert(fmt.Errorf("merge blocked by your working checkout: %s. Commit or stash them (or move/remove them, if untracked), then merge again — the PR is fine and stays approved", prompts.FileList(res.Files)))
	case git.MergeErr:
		return revert(res.Err)
	}
	pr.Status = "merged"
	if err := ps.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	// Any review still out on it is moot, and its reviewer is released and told so — the same thing
	// ScrapPR does. Left open, the reviewer kept being handed a merged PR, read an empty diff, and
	// its rejection overwrote the merge in the record.
	a.ReleaseReviewers(project, prID, "overtaken: merged before a verdict")
	// WHETHER this merge finishes the work is the only thing the merge itself decides. A task that
	// gained a child while its PR was out reads as finished here, and closing it is THE incident.
	partial := pr.Kind == "interim"
	if !partial {
		open, oerr := ps.OpenChildIDs(pr.Task)
		if oerr != nil {
			return store.PR{}, oerr
		}
		partial = len(open) > 0
		// Recorded, not just acted on: a leaf PR that lands over work its task gained IS a milestone,
		// and every later reading of "has this feature landed" asks the row (-> situation's
		// featureLanded). Left as a final merge, it told the author its feature was over.
		if partial {
			pr.Kind = "interim"
			if err := ps.PutPR(pr); err != nil {
				return store.PR{}, err
			}
			_ = ps.LogPR(prID, "milestone", "the task gained children while this was out")
		}
	}
	if partial {
		// What a milestone MEANS for its author is that agent's map to decide, woken by this landing
		// (-> cond.GainedChildren, cond.MilestoneLanded).
		return a.finishPartialMerge(project, pr, false)
	}
	// Every task source is told THIS PR MERGED, so each runs its own consequence on its own ids.
	// After the local merge, so a failure warns rather than fails it.
	note := "merged via " + prID
	for _, src := range a.TaskSources(project) {
		if err := src.OnMerged(root, pr.Task, note); err != nil {
			log.Printf("hub: %s merged locally but a task-source close failed: %v", prID, err)
			_ = ps.LogPR(prID, "warning", "merged locally, but closing the task upstream failed (may need a manual follow-up): "+err.Error())
		}
	}
	// Whether the task is FINISHED is not decided here. Its own map reads a landed final PR with
	// nothing open beneath it and closes itself (-> flow/task's landed) — the same rule the repair
	// sweep applied, in the one place it now lives.
	a.Flow.LookTask(project, pr.Task)
	_ = ps.Log(pr.Agent, "merged", prID)
	_ = ps.LogPR(prID, "merged", "into "+pr.Base)
	// The AUTHOR is not touched: a merge writes the PR's outcome and wakes. Releasing it from here
	// was one function writing four subjects (-> cond.PRSettled).
	a.Flow.WakeProject(project, topic.PRMerged)
	a.rebasePlanners(project, pr.Base) // any merge moves base → keep planners current
	a.Deps.Notify()
	return pr, nil
}

// finishPartialMerge resumes a merged, partial PR's agent — the tail both a clean reset and a
// resolved post-merge reapply conflict end up at, so the two paths can never drift apart.
func (a *Act) finishPartialMerge(project string, pr store.PR, onFeature bool) (store.PR, error) {
	ps := a.Store.For(project)
	if onFeature {
		_ = ps.Log(pr.Agent, "merged", pr.ID+" (milestone)")
		_ = ps.LogPR(pr.ID, "merged", "milestone into "+pr.Base)
		a.resumeContainer(project, pr.Agent)
		// Push only: the agent resumes the same feature it never left, which its own directive
		// already says — nothing here needs to survive being read late.
		_ = a.Harness.Say(project, pr.Agent, prompts.MsgMilestoneMerged(pr.ID), mail.PushOnly)
	} else {
		// Phase only: promoteToFeature only promotes a "working" agent, so this one never picked up
		// a container while its interim PR was out.
		_ = ps.SetPhase(pr.Agent, "working", store.ReasonLanded, "interim merged: "+pr.ID)
		_ = ps.Log(pr.Agent, "merged", pr.ID+" (interim)")
		_ = ps.LogPR(pr.ID, "merged", "interim contribution into "+pr.Base)
		// Push only, same reason: it resumes the same task, which its directive already says.
		_ = a.Harness.Say(project, pr.Agent, prompts.MsgContributionMerged(pr.ID, pr.Task), mail.PushOnly)
	}
	a.rebasePlanners(project, pr.Base) // any merge moves base → keep planners current
	a.Deps.Notify()
	return pr, nil
}
