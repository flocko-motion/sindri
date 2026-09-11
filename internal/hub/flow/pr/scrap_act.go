// package: hub/flow/pr / scrap_act
// type:    logic (PR discard)
// job:     ScrapPR — discard a PR whose task is being closed/scrapped: stop any
// reviewer mid-review, delete the task's branch, and flip the PR to
// "scrapped" so it drops off the board. The worker is stopped by the paired
// task close, not here.
// limits:  git mechanics via adapter/git; persistence via the store. No git/tmux here.
package pr

import (
	"fmt"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"path/filepath"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// DiscardPR scraps a PR on its own, for work the user simply does not want, and releases its
// author. The release is the whole difference from ScrapPR, which leaves that to the paired task
// close: with no close alongside, the author would sit in "submitted" awaiting a verdict on a PR
// that no longer exists.
func (a *Act) DiscardPR(project, prID string) error {
	ps := a.Store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w %q", core.ErrNoSuchPR, prID)
	}
	author := pr.Agent
	if err := a.ScrapPR(project, prID); err != nil {
		return err
	}
	if author == "" {
		return nil
	}
	// Only release an author still waiting on THIS PR. One that has moved on (or never
	// blocked) must not be interrupted for a verdict it isn't expecting.
	st, _ := ps.GetState(author)
	if situation.Standing(st.Phase, "submitted", "resolving") {
		// The interrupt needs it up; the verdict reaches it either way, mail being the half that
		// waits for one that is down.
		if a.Harness.Observe(project, author).Up {
			_ = a.Harness.Interrupt(project, author)
		}
		_ = a.Harness.Say(project, author, prompts.MsgPRScrapped(prID), mail.MailAndPush.From(api.SenderUser))
		// A container holder keeps its FEATURE: an interim or milestone PR being discarded does not
		// mean the feature itself is done (sd-5ef393 — the same shape as FinishTask's own fix). The
		// author's map reads the PR going away and stands it back up (-> cond.PRSettled).
		_ = ps.SetHolding(author, "", st.Container, st.Container, store.ReasonFreed, "PR discarded: "+prID)
	}
	_ = ps.Log(author, "pr-scrapped", prID)
	a.announceHolding(project, author)
	return nil
}

// ScrapPR discards a PR (host-only), the companion to closing its task. It does NOT touch the
// working agent — the paired FinishTask frees it, and doing both would double-message the worker.
func (a *Act) ScrapPR(project, prID string) error {
	ps := a.Store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w %q", core.ErrNoSuchPR, prID)
	}

	// Stop any reviewer mid-review: the branch is about to vanish, so the review is moot. Abort,
	// tell it, close the review record so it stops showing as "reviewing", idle it. Best-effort.
	revs, _ := ps.Reviews(prID)
	for _, r := range revs {
		if r.Verdict != "" {
			continue // already finished — nothing in flight
		}
		if a.Harness.Probe(project, r.Author).Up {
			_ = a.Harness.Interrupt(project, r.Author)
			_ = a.Harness.Say(project, r.Author, prompts.MsgReviewCancelled(prID), mail.MailAndPush)
		}
		// The verdict recorded IS the release: the reviewer's own map reads a review it no longer
		// holds and stands it down (-> cond.ReviewOvertaken).
		_ = ps.RecordVerdict(r.ID, "cancelled", "PR scrapped with its task")
		_ = ps.Log(r.Author, "review-cancelled", prID)
		a.Flow.Wake(project, r.Author, topic.PRVerdict)
	}

	// Discard the work. Best-effort but LOUD: a failure is recorded on the PR rather than
	// leaving the branch as it was, silently.
	disposal := ""
	if pr.Branch != "" {
		wt := ""
		if ag, ok, _ := ps.GetAgent(pr.Agent); ok && ag.Workspace != "" && ag.Workspace != "." {
			wt = filepath.Join(a.Deps.ProjectRoot(project), ag.Workspace)
		}
		var derr error
		disposal, derr = a.discardBranch(project, pr, wt)
		if derr != nil {
			_ = ps.LogPR(prID, "scrap-branch-failed", derr.Error())
		}
	}

	pr.Status = "scrapped"
	if err := ps.PutPR(pr); err != nil {
		return err
	}
	_ = ps.LogPR(prID, "scrapped", "discarded with its task; "+disposal)
	a.Deps.Notify()
	return nil
}

// discardBranch throws away a scrapped PR's work and says what it did. A planner's branch is
// STANDING, so scrapping empties it instead: deleting it detached the worktree to free the name,
// leaving the planner on a HEAD no branch held, committing there and unable to rebase ever again.
func (a *Act) discardBranch(project string, pr store.PR, wt string) (string, error) {
	root := a.Deps.ProjectRoot(project)
	if pr.Branch != core.PlannerBranch(pr.Agent) {
		return "branch " + pr.Branch + " removed", git.ScrapBranch(root, wt, pr.Branch)
	}
	base, err := a.BaseBranch(root)
	if err != nil {
		base = pr.Base // the reference branch moved or is unreadable; the PR's own base still holds
	}
	if wt == "" {
		return "", fmt.Errorf("standing branch %s has no worktree to reset", pr.Branch)
	}
	return "branch " + pr.Branch + " reset to " + base, git.ResetBranchTo(wt, base)
}
