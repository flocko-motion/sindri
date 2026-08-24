// package: hub/workflow / scrap
// type:    logic (PR discard)
// job:     ScrapPR — discard a PR whose task is being closed/scrapped: stop any
// reviewer mid-review, delete the task's branch, and flip the PR to
// "scrapped" so it drops off the board. The worker is stopped by the paired
// task close, not here.
// limits:  git mechanics via hub/repo; persistence via the store. No git/tmux here.
package workflow

import (
	"fmt"
	"github.com/flo-at/sindri/internal/api"
	"path/filepath"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
)

// DiscardPR scraps a PR on its own, for work the user simply does not want, and releases its
// author. The release is the whole difference from ScrapPR, which leaves that to the paired task
// close: with no close alongside, the author would sit in "submitted" awaiting a verdict on a PR
// that no longer exists.
func (e *Engine) DiscardPR(project, prID string) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}
	author := pr.Agent
	if err := e.ScrapPR(project, prID); err != nil {
		return err
	}
	if author == "" {
		return nil
	}
	// Only release an author still waiting on THIS PR. One that has moved on (or never
	// blocked) must not be interrupted for a verdict it isn't expecting.
	st, _ := ps.GetState(author)
	if st.Phase == "submitted" || st.Phase == "resolving" {
		// The interrupt needs it up; the verdict reaches it either way, mail being the half that
		// waits for one that is down.
		if e.deps.AgentUp(project, author) {
			_ = e.deps.Interrupt(project, author)
		}
		_ = e.deps.Deliver(project, author, MsgPRScrapped(prID), MailAndPush.From(api.SenderUser))
		_ = ps.SetState(store.AgentState{Agent: author, Phase: "idle"})
	}
	_ = ps.Log(author, "pr-scrapped", prID)
	e.deps.Notify()
	return nil
}

// ScrapPR discards a PR (host-only), the companion to closing its task. It does NOT touch the
// working agent — the paired finishTask frees it, and doing both would double-message the worker.
func (e *Engine) ScrapPR(project, prID string) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}

	// Stop any reviewer mid-review: the branch is about to vanish, so the review is moot. Abort,
	// tell it, close the review record so it stops showing as "reviewing", idle it. Best-effort.
	revs, _ := ps.Reviews(prID)
	for _, r := range revs {
		if r.Verdict != "" {
			continue // already finished — nothing in flight
		}
		if e.deps.AgentAlive(project, r.Author) {
			_ = e.deps.Interrupt(project, r.Author)
			_ = e.deps.Deliver(project, r.Author, MsgReviewCancelled(prID), MailAndPush)
		}
		_ = ps.RecordVerdict(r.ID, "cancelled", "PR scrapped with its task")
		_ = ps.SetState(store.AgentState{Agent: r.Author, Phase: "idle"})
		_ = ps.Log(r.Author, "review-cancelled", prID)
	}

	// Discard the work. Best-effort but LOUD: a failure is recorded on the PR rather than
	// leaving the branch as it was, silently.
	disposal := ""
	if pr.Branch != "" {
		wt := ""
		if a, ok, _ := ps.GetAgent(pr.Agent); ok && a.Workspace != "" && a.Workspace != "." {
			wt = filepath.Join(e.deps.ProjectRoot(project), a.Workspace)
		}
		var derr error
		disposal, derr = e.discardBranch(project, pr, wt)
		if derr != nil {
			_ = ps.LogPR(prID, "scrap-branch-failed", derr.Error())
		}
	}

	pr.Status = "scrapped"
	if err := ps.PutPR(pr); err != nil {
		return err
	}
	_ = ps.LogPR(prID, "scrapped", "discarded with its task; "+disposal)
	e.deps.Notify()
	return nil
}

// discardBranch throws away a scrapped PR's work and says what it did. A planner's branch is
// STANDING, so scrapping empties it instead: deleting it detached the worktree to free the name,
// leaving the planner on a HEAD no branch held, committing there and unable to rebase ever again.
func (e *Engine) discardBranch(project string, pr store.PR, wt string) (string, error) {
	root := e.deps.ProjectRoot(project)
	if pr.Branch != PlannerBranch(pr.Agent) {
		return "branch " + pr.Branch + " removed", repo.ScrapBranch(root, wt, pr.Branch)
	}
	base, err := e.baseBranch(root)
	if err != nil {
		base = pr.Base // the reference branch moved or is unreadable; the PR's own base still holds
	}
	if wt == "" {
		return "", fmt.Errorf("standing branch %s has no worktree to reset", pr.Branch)
	}
	return "branch " + pr.Branch + " reset to " + base, git.ResetBranchTo(wt, base)
}
