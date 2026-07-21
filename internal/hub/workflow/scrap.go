// package: hub/workflow / scrap
// type:    logic (PR discard)
// job:     ScrapPR — discard a PR whose task is being closed/scrapped: stop any
//          reviewer mid-review, delete the task's branch, and flip the PR to
//          "scrapped" so it drops off the board. The worker is stopped by the paired
//          task close, not here.
// limits:  git mechanics via hub/repo; persistence via the store. No git/tmux here.
package workflow

import (
	"fmt"
	"path/filepath"

	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
)

// ScrapPR discards a PR (host/human-only) — the companion to closing/scrapping its
// task when the human decides the work isn't wanted. It deletes the task's branch
// (unpushed local work on it is intentionally discarded) and marks the PR "scrapped"
// so it leaves the board. It does NOT touch the working agent: the paired task close
// (finishTask) interrupts and frees it, and pairing them here would double-message the
// worker. A missing PR is an error; a branch that's already gone is fine (logged).
func (e *Engine) ScrapPR(project, prID string) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}

	// Stop any reviewer mid-review of this PR — its branch is about to vanish, so the
	// review is moot. Abort the reviewer's current op (ESC), tell it, close the open
	// review record so it stops showing as "reviewing", and idle it. (The worker on the
	// task is handled by the paired task close.) Best-effort per reviewer.
	revs, _ := ps.Reviews(prID)
	for _, r := range revs {
		if r.Verdict != "" {
			continue // already finished — nothing in flight
		}
		if e.deps.AgentAlive(project, r.Author) {
			_ = e.deps.Interrupt(project, r.Author)
			_ = e.deps.InjectWhenReady(project, r.Author, MsgReviewCancelled(prID))
		}
		_ = ps.RecordVerdict(r.ID, "cancelled", "PR scrapped with its task")
		_ = ps.SetState(store.AgentState{Agent: r.Author, Phase: "idle"})
		_ = ps.Log(r.Author, "review-cancelled", prID)
	}

	// Delete the task's branch. It's usually checked out in the owning agent's
	// worktree, so ScrapBranch detaches that first. Best-effort but LOUD: a failure is
	// recorded on the PR rather than leaving a lingering branch silently.
	if pr.Branch != "" {
		wt := ""
		if a, ok, _ := ps.GetAgent(pr.Agent); ok && a.Workspace != "" && a.Workspace != "." {
			wt = filepath.Join(e.deps.ProjectRoot(project), a.Workspace)
		}
		if derr := repo.ScrapBranch(e.deps.ProjectRoot(project), wt, pr.Branch); derr != nil {
			_ = ps.LogPR(prID, "scrap-branch-failed", derr.Error())
		}
	}

	pr.Status = "scrapped"
	if err := ps.PutPR(pr); err != nil {
		return err
	}
	_ = ps.LogPR(prID, "scrapped", "discarded with its task; branch "+pr.Branch+" removed")
	e.deps.Notify()
	return nil
}
