// package: hub/workflow / review
// type:    logic (assigning a review and holding it)
// job:     give ONE reviewer ONE pull request and put that PR's branch in its
// workspace — the assignment, the hold that keeps it, and the release when
// the PR is settled before a verdict arrives.
// limits:  who reviews what; the verdicts themselves are pr.go's (approve/reject).
package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// ReviewPrompt reads review-prompt.txt, auto-created from a built-in default if absent.
func (e *Engine) ReviewPrompt(project string) (string, error) {
	// A repo-committed `review_prompt` wins; config already validated the path exists.
	if cfg, err := e.deps.ProjectConfig(project); err != nil {
		return "", err
	} else if cfg.ReviewPrompt != "" {
		data, rerr := os.ReadFile(config.Abs(e.deps.ProjectRoot(project), cfg.ReviewPrompt))
		if rerr != nil {
			return "", fmt.Errorf("read review_prompt %s: %w", cfg.ReviewPrompt, rerr)
		}
		return strings.TrimSpace(string(data)), nil
	}
	dir := filepath.Join(paths.StateDir(), project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "review-prompt.txt")
	if data, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(data)), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(path, []byte(DefaultReviewPrompt+"\n"), 0o644); err != nil {
		return "", err
	}
	return DefaultReviewPrompt, nil
}

// RequestReview is the ONE review path: every trigger funnels here, so a review is always
// the same thing. No reviewer running → recorded unassigned; requirement "" uses the default.
func (e *Engine) RequestReview(project, prID, requirement string) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}
	requirement = strings.TrimSpace(requirement)
	if requirement == "" {
		requirement, _ = e.ReviewPrompt(project)
	}
	// Asking again with new instructions unmakes an approval: the verdict answered the previous
	// question, and the PR has to be open for anyone to be handed it.
	if pr.Status == "approved" {
		pr.Status = "open"
		if err := ps.PutPR(pr); err != nil {
			return err
		}
		_ = ps.LogPR(prID, "reopened", "a fresh review was requested, so the approval no longer stands")
	}
	// A reviewer already on this PR is told MORE, rather than a second review being opened: it holds
	// the branch, so the instructions belong to the agent looking at it.
	if id, holder := e.reviewerHolding(project, prID); holder != "" {
		if err := ps.AmendReview(id, requirement); err != nil {
			return err
		}
		_ = ps.LogPR(prID, "review-amended", "further instructions to "+holder)
		go e.deps.InjectWhenReady(project, holder, MsgReviewAmended(prID, requirement))
		e.deps.Notify()
		return nil
	}
	id, err := ps.AddReview(prID, requirement)
	if err != nil {
		return err
	}
	reviewer, err := e.freeReviewer(project)
	if err != nil {
		return err
	}
	if reviewer == "" {
		// Left for whichever reviewer frees up (-> UnclaimedReview). Handing it to one that is
		// mid-review would check the new branch out over the one it is reading: a reviewer has one
		// workspace, so it can hold exactly one PR, and a second assignment is not a queue.
		_ = ps.LogPR(prID, "review-requested", "unassigned (no free reviewer)")
		e.deps.Notify()
		return nil
	}
	if err := e.assignReview(project, id, prID, reviewer, requirement); err != nil {
		return err
	}
	return nil
}

// assignReview gives one reviewer one PR: the review record, the branch in its workspace, and the
// instruction — in that order, so what it is told to review is what it is looking at.
func (e *Engine) assignReview(project string, id int64, prID, reviewer, requirement string) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil || !ok {
		return err
	}
	if err := ps.AssignReview(id, reviewer); err != nil {
		return err
	}
	// The hub preps the terrain so the reviewer never faces a stale tree: force-checkout is
	// safe because it only reads + lints. On failure it is told not to trust /workspace.
	checkedOut := true
	if a, ok, gerr := ps.GetAgent(reviewer); gerr != nil || !ok {
		checkedOut = false
		_ = ps.LogPR(prID, "checkout-failed", "reviewer "+reviewer+" not on roster")
	} else if coErr := git.CheckoutDetachedClean(filepath.Join(e.deps.ProjectRoot(project), a.Workspace), pr.Branch); coErr != nil {
		checkedOut = false
		_ = ps.LogPR(prID, "checkout-failed", fmt.Sprintf("%s into %s: %v", pr.Branch, a.Workspace, coErr))
	}
	_ = ps.SetState(store.AgentState{Agent: reviewer, Phase: "reviewing"}) // board shows it working, not idle
	_ = ps.LogPR(prID, "review-requested", "assigned to "+reviewer)
	go e.deps.InjectWhenReady(project, reviewer, MsgReview(prID, requirement, pr.Branch, pr.Base, e.deps.ArchitectureDoc(project), checkedOut)) // async: don't block a worker's submit
	e.deps.Notify()
	return nil
}

// reviewDirective is what a reviewer is told: the ONE PR it holds, whose branch is what sits in its
// workspace. A reviewer has one checkout, so it can review one PR — scanning the open PRs and
// serving whichever came first handed it a second while the first was still on disk, and the diff
// it read then belonged to neither. Free, it claims the oldest unclaimed review, which is also how
// one requested with no reviewer running is eventually picked up.
func (e *Engine) reviewDirective(project, name string) (string, bool, error) {
	ps := e.store.For(project)
	held, err := ps.ReviewingPR(name)
	if err != nil {
		return "", false, err
	}
	if held != "" {
		pr, ok, err := ps.GetPR(held)
		if err != nil {
			return "", false, err
		}
		if ok && pr.Status == "open" {
			return DirReview(pr.ID, pr.Task, e.deps.ArchitectureDoc(project)), true, nil
		}
		// Settled while it was reading: a verdict on it now decides nothing, so the hold is released
		// rather than left to produce one.
		if err := ps.CloseReviews(held, "overtaken: the PR was "+pr.Status+" before a verdict"); err != nil {
			return "", false, err
		}
		_ = ps.SetState(store.AgentState{Agent: name, Phase: restPhase("reviewer")})
		_ = e.deps.InjectWhenReady(project, name, MsgReviewCancelled(held))
	}
	var id int64
	var prID string
	found, err := ps.UnclaimedReview(&id, &prID)
	if err != nil || !found {
		return "", false, err
	}
	req, _ := e.ReviewPrompt(project)
	if err := e.assignReview(project, id, prID, name, req); err != nil {
		return "", false, err
	}
	pr, _, _ := ps.GetPR(prID)
	return DirReview(prID, pr.Task, e.deps.ArchitectureDoc(project)), true, nil
}

// releaseReviewers closes every open review of a PR and frees whoever held one, telling them the PR
// is settled. Called wherever a PR reaches a terminal state, so no reviewer is left holding a
// verdict that can no longer mean anything.
func (e *Engine) releaseReviewers(project, prID, why string) {
	ps := e.store.For(project)
	revs, _ := ps.Reviews(prID)
	if err := ps.CloseReviews(prID, why); err != nil {
		fmt.Fprintf(os.Stderr, "hub: closing reviews of %s: %v\n", prID, err)
		return
	}
	for _, r := range revs {
		if r.Author == "" || r.Verdict != "" {
			continue
		}
		_ = ps.SetState(store.AgentState{Agent: r.Author, Phase: restPhase("reviewer")})
		_ = e.deps.InjectWhenReady(project, r.Author, MsgReviewCancelled(prID))
	}
	e.deps.Notify()
}

// reviewerHolding returns the open review of prID and who is doing it, or (0, "") if nobody is.
func (e *Engine) reviewerHolding(project, prID string) (int64, string) {
	revs, err := e.store.For(project).Reviews(prID)
	if err != nil {
		return 0, ""
	}
	for _, r := range revs {
		if r.Verdict == "" && r.Author != "" {
			return r.ID, r.Author
		}
	}
	return 0, ""
}

// freeReviewer returns a running reviewer that is holding no review. A roster read failure is
// returned rather than disguised as "no reviewer", which would silently drop the request.
func (e *Engine) freeReviewer(project string) (string, error) {
	ps := e.store.For(project)
	roster, err := ps.Roster()
	if err != nil {
		return "", fmt.Errorf("load roster for %s: %w", project, err)
	}
	for _, a := range roster {
		if a.Role != "reviewer" || !container.Running(e.deps.Container(project, a.Name)) || !e.deps.SessionAlive(project, a.Name) {
			continue
		}
		held, err := ps.ReviewingPR(a.Name)
		if err != nil {
			return "", err
		}
		if held == "" {
			return a.Name, nil
		}
	}
	return "", nil
}
