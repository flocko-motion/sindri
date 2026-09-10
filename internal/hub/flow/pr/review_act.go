// package: hub/flow/pr / review_act
// type:    logic (assigning a review and holding it)
// job:     give ONE reviewer ONE pull request and put that PR's branch in its
// workspace — the assignment, the hold that keeps it, and the release when
// the PR is settled before a verdict arrives.
// limits:  who reviews what; the verdicts themselves are verdict_act.go's (approve/reject).
package pr

import (
	"context"
	"fmt"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// ReviewPrompt is the review instruction: a repo-committed `review_prompt`, else an edited
// review-prompt.txt, else the built-in default.
//
// It does NOT write the default out. Seeding the file on first use meant the default could never be
// improved again: every project that had ever requested a review already held a copy, so a better
// one shipped to new installs only — silently, since nothing reports a stale seed.
func (a *Act) ReviewPrompt(project string) (string, error) {
	// A repo-committed `review_prompt` wins; config already validated the path exists.
	if cfg, err := a.Deps.ProjectConfig(project); err != nil {
		return "", err
	} else if cfg.ReviewPrompt != "" {
		data, rerr := os.ReadFile(config.Abs(a.Deps.ProjectRoot(project), cfg.ReviewPrompt))
		if rerr != nil {
			return "", fmt.Errorf("read review_prompt %s: %w", cfg.ReviewPrompt, rerr)
		}
		return strings.TrimSpace(string(data)), nil
	}
	data, err := os.ReadFile(ReviewPromptPath(project))
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		return prompts.DefaultReviewPrompt, nil
	}
	if text := strings.TrimSpace(string(data)); !IsSeededPrompt(text) {
		return text, nil // someone edited it; their words win
	}
	// A file matching a default sindri itself wrote is the seed, not a choice — so the current
	// default wins and the improvement reaches the installs that already had one.
	return prompts.DefaultReviewPrompt, nil
}

// ReviewPromptPath is where a project's edited review instruction lives.
func ReviewPromptPath(project string) string {
	return filepath.Join(paths.StateDir(), project, "review-prompt.txt")
}

// seededPrompts are the instructions sindri has written into review-prompt.txt itself, current and
// superseded. A file byte-matching one of them was never a decision, so it does not outrank the
// built-in — which is what lets an improved default reach a project that already has the file.
var seededPrompts = []string{
	prompts.DefaultReviewPrompt,
	// Superseded: the one-liner seeded before the reviewer could read the task at all.
	"Review this PR for correctness, clarity, and fit to the task. Flag bugs, missing tests, and anything that should change.",
}

// IsSeededPrompt reports whether text is one sindri wrote rather than one someone chose.
func IsSeededPrompt(text string) bool {
	for _, p := range seededPrompts {
		if text == strings.TrimSpace(p) {
			return true
		}
	}
	return false
}

// TaskTitle is a task's title for a directive, or "" when it cannot be read. Best-effort by design:
// a title that will not load must not stop a review being handed out.
func (a *Act) TaskTitle(project, id string) string {
	if id == "" {
		return ""
	}
	t, ok, err := a.Store.For(project).GetTask(id)
	if err != nil || !ok {
		return ""
	}
	return t.Title
}

// RequestReview is the ONE review path: every trigger funnels here, so a review is always
// the same thing. No reviewer running → recorded unassigned; requirement "" uses the default.
func (a *Act) RequestReview(project, prID, requirement string) error {
	ps := a.Store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w %q", core.ErrNoSuchPR, prID)
	}
	requirement = strings.TrimSpace(requirement)
	if requirement == "" {
		requirement, _ = a.ReviewPrompt(project)
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
	if id, holder := a.ReviewerHolding(project, prID); holder != "" {
		if err := ps.AmendReview(id, requirement); err != nil {
			return err
		}
		_ = ps.LogPR(prID, "review-amended", "further instructions to "+holder)
		go a.Harness.Say(project, holder, prompts.MsgReviewAmended(prID, requirement), core.MailAndPush.From(api.SenderUser))
		a.Deps.Notify()
		return nil
	}
	id, err := ps.AddReview(prID, requirement)
	if err != nil {
		return err
	}
	reviewer, err := a.freeReviewer(project)
	if err != nil {
		return err
	}
	if reviewer == "" {
		// Left for whichever reviewer frees up (-> UnclaimedReview). Handing it to one that is
		// mid-review would check the new branch out over the one it is reading: a reviewer has one
		// workspace, so it can hold exactly one PR, and a second assignment is not a queue.
		_ = ps.LogPR(prID, "review-requested", "unassigned (no free reviewer)")
		a.Deps.Notify()
		return nil
	}
	// The hub's lifetime: a submit reaches here through a port carrying no context of its own.
	if err := a.AssignReview(a.Lifetime, project, id, prID, reviewer, requirement); err != nil {
		return err
	}
	return nil
}

// AssignReview gives one reviewer one PR. The review record stays with the PR's project; the
// reviewer's own roster row, workspace, state and notes are read and written under its own home.
// It does NOT prepare the session: the reviewer's own map clears on its way to taking a review
// (-> hub/flow/roles/reviewer), so a hand-over that also prepared would be a second decider, and two
// of them clear the session twice.
func (a *Act) AssignReview(ctx context.Context, project string, id int64, prID, reviewer, requirement string) (err error) {
	ps := a.Store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil || !ok {
		return err
	}
	if err := ps.AssignReview(id, reviewer); err != nil {
		return err
	}
	home, ag, found := a.reviewerHome(project, reviewer)
	hs := a.Store.For(home)
	// A review IS a reviewer's claim: it has been somewhere and looked at a whole diff, which is the
	// vantage point the note grant pays for (-> store.GrantNotes). Its subsystems are often nobody's
	// task, so this is the role most likely to notice something with no other home.
	if err := hs.GrantNotes(reviewer, prompts.NotesPerClaim); err != nil {
		return err
	}
	// The hub preps the terrain so the reviewer never faces a stale tree; on failure it is told
	// not to trust /workspace. A api.GlobalProject reviewer gets plain files instead (-> git.ArchiveTree).
	checkedOut := true
	dest := filepath.Join(a.Deps.ProjectRoot(home), ag.Workspace)
	switch {
	case !found:
		checkedOut = false
		_ = ps.LogPR(prID, "checkout-failed", "reviewer "+reviewer+" not on roster")
	case home == api.GlobalProject:
		if mErr := git.ArchiveTree(a.Deps.ProjectRoot(project), pr.Branch, dest); mErr != nil {
			checkedOut = false
			_ = ps.LogPR(prID, "checkout-failed", fmt.Sprintf("%s into %s: %v", pr.Branch, dest, mErr))
		}
	default:
		if coErr := git.CheckoutDetachedClean(dest, pr.Branch); coErr != nil {
			checkedOut = false
			_ = ps.LogPR(prID, "checkout-failed", fmt.Sprintf("%s into %s: %v", pr.Branch, ag.Workspace, coErr))
		}
	}
	_ = ps.LogPR(prID, "review-requested", "assigned to "+reviewer)
	msg := prompts.MsgReview(prID, requirement, pr.Branch, pr.Base, a.Deps.ArchitectureDoc(project), checkedOut)
	go a.Harness.Say(home, reviewer, msg, core.MailAndPush) // async: don't block a worker's submit
	a.Deps.Notify()
	return nil
}

// reviewerHome resolves a reviewer's own roster row: its own project first, else api.GlobalProject's.
func (a *Act) reviewerHome(project, reviewer string) (home string, ag store.Agent, ok bool) {
	if ag, ok, err := a.Store.For(project).GetAgent(reviewer); err == nil && ok {
		return project, ag, true
	}
	if project == api.GlobalProject {
		return project, store.Agent{}, false
	}
	ag, ok, err := a.Store.For(api.GlobalProject).GetAgent(reviewer)
	if err != nil {
		return project, store.Agent{}, false
	}
	return api.GlobalProject, ag, ok
}
