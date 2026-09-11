// package: hub/flow/pr / merge_act
// type:    logic (merge workflow)
// job:     the human-gated merge of an approved PR into its base — the intent a human records, and
// the rebase-and-commit the merging state runs behind it, with conflicts routed to the author.
// limits:  merge only; review and submit live beside this (review_act.go, pr_act.go). Where the
// merge intent STANDS is the map's (-> pr.go), and this writes none of it.
package pr

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/store"
	"github.com/flo-at/sindri/internal/hub/world/task"

	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// Merge records a human's intent to merge an approved PR and settles the merge intent, so the caller
// is answered after the merge has run rather than before. The merge itself happens in pr/merging —
// a merge is not atomic, and an intent that survives a restart is what makes a hub dying half way
// through recoverable rather than silent.
func (a *Act) Merge(project, prID string) (store.PR, error) {
	ps := a.Store.For(project)
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
	if err := ps.SetMergeAsked(prID, true); err != nil {
		return store.PR{}, err
	}
	_ = ps.LogPR(prID, "merge-asked", "a human asked for the merge")
	a.Deps.Notify()
	a.Flow.LookPR(project, prID)
	merged, _, err := ps.GetPR(prID)
	if err != nil {
		return store.PR{}, err
	}
	switch merged.Status {
	case "merged":
		return merged, nil
	case "merge-failed":
		return store.PR{}, fmt.Errorf("%s is half merged — a hub died mid-merge, so whether %s carries it needs a human's eye",
			prID, merged.Base)
	case "open":
		// A conflict sends it back to its author, which is what "open" means here. The reason is on
		// the record, which is where every other reader of this merge looks too.
		return store.PR{}, fmt.Errorf("%s conflicts with %s — sent to %s to resolve; it returns for review once clean",
			prID, merged.Base, merged.Agent)
	}
	return store.PR{}, fmt.Errorf("%s did not merge: %s", prID, a.lastMergeFailure(ps, prID))
}

// lastMergeFailure is the reason the merge did not run, off the PR's own record — the words the
// action wrote when it refused, rather than a second account carried back through a return value.
func (a *Act) lastMergeFailure(ps *store.ProjectStore, prID string) string {
	events, err := ps.PREvents(prID)
	if err != nil {
		return "the reason is on its record"
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == "merge-refused" {
			return events[i].Payload
		}
	}
	return "the reason is on its record"
}

// RunMerge is what pr/merging does: rebase the branch onto its base and commit it there, then route
// the outcome. It takes the request back first — one left standing would bring the pull request
// straight back here on the next beat, whatever this merge then did.
func (a *Act) RunMerge(_ context.Context, w World) (Outcome, error) {
	ps := a.Store.For(w.Project)
	_ = ps.SetMergeAsked(w.ID, false)
	root := a.Deps.ProjectRoot(w.Project)
	pr, ok, err := ps.GetPR(w.ID)
	if err != nil || !ok {
		return Refused, err
	}
	// Mechanics live in the adapter; here we route the outcome. The rebase runs in the AUTHOR's
	// worktree, and a conflict goes into its resolution loop rather than a dead-end "resubmit".
	wt, workspace := "", ""
	if ag, found, _ := ps.GetAgent(pr.Agent); found {
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
		// Recorded on the PR, not written onto its author: a conflict is a fact about this merge
		// intent, and what it means for whoever filed it is their map's (-> cond.MergeConflicted).
		pr.Feedback = "" // no longer mergeable; back to review once the author has resolved it
		_ = ps.PutPR(pr)
		_ = ps.LogPR(pr.ID, "conflict", "rebase onto "+pr.Base+" conflicts: "+strings.Join(res.Files, ", "))
		a.Flow.WakeProject(w.Project, topic.PRVerdict)
		_ = a.Harness.Say(w.Project, pr.Agent, prompts.MsgResolveNeeded(pr.Base, res.Files), mail.MailAndPush)
		a.Deps.Notify()
		return Conflicted, nil
	case git.MergeRebaseErr:
		// Name the worktree: it is the AGENT's, and "unstaged changes" otherwise sends you hunting
		// through your own checkout for edits the agent left in its.
		return a.refuse(ps, pr.ID, fmt.Sprintf("can't rebase %s onto %s in %s's worktree (%s) — most often it has uncommitted changes (NOT your checkout); have the agent commit or discard them, e.g. `sindri agent tell %s \"commit or discard your /workspace changes, then say done\"`. git said: %v",
			pr.Branch, pr.Base, pr.Agent, workspace, pr.Agent, res.Err))
	case git.MergeBlocked:
		// "commit or stash" alone dead-ends an untracked collision: you cannot stash an untracked file.
		return a.refuse(ps, pr.ID, "merge blocked by your working checkout: "+prompts.FileList(res.Files)+
			". Commit or stash them (or move/remove them, if untracked), then merge again — the PR is fine and stays approved")
	case git.MergeErr:
		return a.refuse(ps, pr.ID, res.Err.Error())
	}
	return a.landed(w.Project, pr)
}

// refuse records why a merge could not run and leaves the pull request approved, so it stays
// retryable and the words say what to fix.
func (a *Act) refuse(ps *store.ProjectStore, prID, why string) (Outcome, error) {
	_ = ps.LogPR(prID, "merge-refused", why)
	a.Deps.Notify()
	return Refused, nil
}

// landed is everything a merge that went in sets off. The PR's own status is NOT written here — the
// state it lands in claims it (-> fleet's movePRState) — and every other subject is woken rather
// than reached into.
func (a *Act) landed(project string, pr store.PR) (Outcome, error) {
	ps := a.Store.For(project)
	root := a.Deps.ProjectRoot(project)
	// Any review still out on it is moot, and its reviewer is released and told so — the same thing
	// ScrapPR does. Left open, the reviewer kept being handed a merged PR, read an empty diff, and
	// its rejection overwrote the merge in the record.
	a.ReleaseReviewers(project, pr.ID, "overtaken: merged before a verdict")
	// WHETHER this merge finishes the work is the only thing the merge itself decides. A task that
	// gained a child while its PR was out reads as finished here, and closing it is THE incident.
	partial := pr.Kind == "interim"
	if !partial {
		open, oerr := ps.OpenChildIDs(pr.Task)
		if oerr != nil {
			return Landed, oerr
		}
		partial = len(open) > 0
		// Recorded, not just acted on: a leaf PR that lands over work its task gained IS a milestone,
		// and every later reading of "has this feature landed" asks the row (-> situation's
		// featureLanded). Left as a final merge, it told the author its feature was over.
		if partial {
			pr.Kind = "interim"
			if err := ps.PutPR(pr); err != nil {
				return Landed, err
			}
			_ = ps.LogPR(pr.ID, "milestone", "the task gained children while this was out")
		}
	}
	if partial {
		// What a milestone MEANS for its author is that agent's map to decide, woken by this landing
		// (-> cond.GainedChildren, cond.MilestoneLanded).
		a.finishPartialMerge(project, pr, false)
		return Landed, nil
	}
	// Every task source is told THIS PR MERGED, so each runs its own consequence on its own ids.
	// After the local merge, so a failure warns rather than fails it.
	note := "merged via " + pr.ID
	for _, src := range a.TaskSources(project) {
		if err := src.OnMerged(root, pr.Task, note); err != nil {
			log.Printf("hub: %s merged locally but a task-source close failed: %v", pr.ID, err)
			_ = ps.LogPR(pr.ID, "warning", "merged locally, but closing the task upstream failed (may need a manual follow-up): "+err.Error())
		}
	}
	// Whether the task is FINISHED is not decided here. Its own map reads a landed final PR with
	// nothing open beneath it and closes itself (-> flow/task's landed).
	a.Flow.LookTask(project, pr.Task)
	_ = ps.Log(pr.Agent, "merged", pr.ID)
	// The AUTHOR is not touched: a merge writes the PR's outcome and wakes. Releasing it from here
	// was one function writing four subjects (-> cond.PRSettled).
	a.Flow.WakeProject(project, topic.PRMerged)
	a.rebasePlanners(project, pr.Base) // any merge moves base → keep planners current
	a.Deps.Notify()
	return Landed, nil
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
		_ = ps.Log(pr.Agent, "merged", pr.ID+" (interim)")
		_ = ps.LogPR(pr.ID, "merged", "interim contribution into "+pr.Base)
		// The author's own map puts it back on the work it never let go of (-> cond.MilestoneLanded);
		// this only wakes it. Push only, same reason: it resumes the same task, which its directive
		// already says.
		a.Flow.Wake(project, pr.Agent, topic.PRMerged)
		_ = a.Harness.Say(project, pr.Agent, prompts.MsgContributionMerged(pr.ID, pr.Task), mail.PushOnly)
	}
	a.rebasePlanners(project, pr.Base) // any merge moves base → keep planners current
	a.Deps.Notify()
	return pr, nil
}
