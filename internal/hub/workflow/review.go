// package: hub/workflow / review
// type:    logic (assigning a review and holding it)
// job:     give ONE reviewer ONE pull request and put that PR's branch in its
// workspace — the assignment, the hold that keeps it, and the release when
// the PR is settled before a verdict arrives.
// limits:  who reviews what; the verdicts themselves are pr.go's (approve/reject).
package workflow

import (
	"fmt"
	"github.com/flo-at/sindri/internal/api"
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
	data, err := os.ReadFile(reviewPromptPath(project))
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		return DefaultReviewPrompt, nil
	}
	if text := strings.TrimSpace(string(data)); !isSeededPrompt(text) {
		return text, nil // someone edited it; their words win
	}
	// A file matching a default sindri itself wrote is the seed, not a choice — so the current
	// default wins and the improvement reaches the installs that already had one.
	return DefaultReviewPrompt, nil
}

// reviewPromptPath is where a project's edited review instruction lives.
func reviewPromptPath(project string) string {
	return filepath.Join(paths.StateDir(), project, "review-prompt.txt")
}

// seededPrompts are the instructions sindri has written into review-prompt.txt itself, current and
// superseded. A file byte-matching one of them was never a decision, so it does not outrank the
// built-in — which is what lets an improved default reach a project that already has the file.
var seededPrompts = []string{
	DefaultReviewPrompt,
	// Superseded: the one-liner seeded before the reviewer could read the task at all.
	"Review this PR for correctness, clarity, and fit to the task. Flag bugs, missing tests, and anything that should change.",
}

// isSeededPrompt reports whether text is one sindri wrote rather than one someone chose.
func isSeededPrompt(text string) bool {
	for _, p := range seededPrompts {
		if text == strings.TrimSpace(p) {
			return true
		}
	}
	return false
}

// taskTitle is a task's title for a directive, or "" when it cannot be read. Best-effort by design:
// a title that will not load must not stop a review being handed out.
func (e *Engine) taskTitle(project, id string) string {
	if id == "" {
		return ""
	}
	t, ok, err := e.store.For(project).GetTask(id)
	if err != nil || !ok {
		return ""
	}
	return t.Title
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
		return fmt.Errorf("%w %q", ErrNoSuchPR, prID)
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
		go e.deps.Deliver(project, holder, MsgReviewAmended(prID, requirement), MailAndPush.From(api.SenderUser))
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

// assignReview gives one reviewer one PR. The review record stays with the PR's project; the
// reviewer's own roster row, workspace, state and notes are read and written under its own home.
func (e *Engine) assignReview(project string, id int64, prID, reviewer, requirement string) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil || !ok {
		return err
	}
	if err := ps.AssignReview(id, reviewer); err != nil {
		return err
	}
	home, a, found := e.reviewerHome(project, reviewer)
	hs := e.store.For(home)
	// A review IS a reviewer's claim: it has been somewhere and looked at a whole diff, which is the
	// vantage point the note grant pays for (-> store.GrantNotes). Its subsystems are often nobody's
	// task, so this is the role most likely to notice something with no other home.
	if err := hs.GrantNotes(reviewer, NotesPerClaim); err != nil {
		return err
	}
	// The hub preps the terrain so the reviewer never faces a stale tree; on failure it is told
	// not to trust /workspace. A GlobalProject reviewer gets plain files instead (-> git.ArchiveTree).
	checkedOut := true
	dest := filepath.Join(e.deps.ProjectRoot(home), a.Workspace)
	switch {
	case !found:
		checkedOut = false
		_ = ps.LogPR(prID, "checkout-failed", "reviewer "+reviewer+" not on roster")
	case home == GlobalProject:
		if mErr := git.ArchiveTree(e.deps.ProjectRoot(project), pr.Branch, dest); mErr != nil {
			checkedOut = false
			_ = ps.LogPR(prID, "checkout-failed", fmt.Sprintf("%s into %s: %v", pr.Branch, dest, mErr))
		}
	default:
		if coErr := git.CheckoutDetachedClean(dest, pr.Branch); coErr != nil {
			checkedOut = false
			_ = ps.LogPR(prID, "checkout-failed", fmt.Sprintf("%s into %s: %v", pr.Branch, a.Workspace, coErr))
		}
	}
	_ = hs.SetState(store.AgentState{Agent: reviewer, Phase: "reviewing"}) // board shows it working, not idle
	_ = ps.LogPR(prID, "review-requested", "assigned to "+reviewer)
	go e.deps.Deliver(home, reviewer, MsgReview(prID, requirement, pr.Branch, pr.Base, e.deps.ArchitectureDoc(project), checkedOut), MailAndPush) // async: don't block a worker's submit
	e.deps.Notify()
	return nil
}

// reviewerHome resolves a reviewer's own roster row: its own project first, else GlobalProject's.
func (e *Engine) reviewerHome(project, reviewer string) (home string, a store.Agent, ok bool) {
	if a, ok, err := e.store.For(project).GetAgent(reviewer); err == nil && ok {
		return project, a, true
	}
	if project == GlobalProject {
		return project, store.Agent{}, false
	}
	a, ok, err := e.store.For(GlobalProject).GetAgent(reviewer)
	if err != nil {
		return project, store.Agent{}, false
	}
	return GlobalProject, a, ok
}

// reviewDirective is what a reviewer is told: the ONE PR it holds, whose branch sits in its one
// workspace — serving a second while the first was still checked out left neither diff readable.
// Free, it claims the oldest unclaimed review, also how one with no reviewer running gets picked up.
func (e *Engine) reviewDirective(project, name string) (string, bool, error) {
	ps := e.store.For(project)
	// heldProject, not project: a GlobalProject reviewer's held review is filed under whatever
	// project it was sent to, never its own.
	heldProject, held, err := e.store.ReviewingPR(project, name)
	if err != nil {
		return "", false, err
	}
	if held != "" {
		hps := e.store.For(heldProject)
		pr, ok, err := hps.GetPR(held)
		if err != nil {
			return "", false, err
		}
		if ok && pr.Status == "open" {
			return DirReview(pr.ID, pr.Task, e.taskTitle(heldProject, pr.Task), pr.Agent, e.deps.ArchitectureDoc(heldProject)), true, nil
		}
		// Settled while it was reading: a verdict on it now decides nothing, so the hold is released
		// rather than left to produce one.
		if err := hps.CloseReviews(held, "overtaken: the PR was "+pr.Status+" before a verdict"); err != nil {
			return "", false, err
		}
		_ = ps.SetState(store.AgentState{Agent: name, Phase: restPhase("reviewer")})
		_ = e.deps.Deliver(project, name, MsgReviewCancelled(held), MailAndPush)
	}
	var id int64
	var prID string
	found, err := ps.UnclaimedReview(&id, &prID)
	if err != nil {
		return "", false, err
	}
	if !found {
		// `sindri` answers at once: AssignPendingReviews pushes a wake once a review is claimable.
		return DirNoReviews, true, nil
	}
	// An armed clear preempts the claim below, firing eagerly rather than waiting for the sweep, same
	// as fireClearIfArmed — interrupt=false for the same reason: this runs inside the reviewer's own ask.
	if e.clearArmed(project, name) {
		if err := e.deps.FireClear(project, name, MsgKickoff, false); err != nil {
			return "", false, err
		}
		return DirClearPending, true, nil // about to land: a review claimed now would be cut in half by it
	}
	// Claim FIRST — same reason claimNext claims before it prepares: once the review is the
	// reviewer's, no return in the middle is needed for compaction (a review has no tier, so no
	// model switch) to run against it.
	req, _ := e.ReviewPrompt(project)
	if err := e.assignReview(project, id, prID, name, req); err != nil {
		return "", false, err
	}
	pr, _, _ := ps.GetPR(prID)
	dir := DirReview(prID, pr.Task, e.taskTitle(project, pr.Task), pr.Agent, e.deps.ArchitectureDoc(project))
	e.deps.BeginAssignment(project, name)
	fired, err := e.compactIfDue(project, name, dir)
	e.deps.EndAssignment(project, name)
	if err != nil {
		return "", false, err
	}
	if fired {
		return DirPreparing, true, nil
	}
	return dir, true, nil
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
		_ = e.deps.Deliver(project, r.Author, MsgReviewCancelled(prID), MailAndPush)
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

// reviewerAssignable reports whether a roster row may be handed a review, from the row alone. An
// armed clear disqualifies one: PR after PR would defer it for ever, and an assignment slipping in
// while the tick fires the clear would clear a reviewer mid-review. (Retirement: -> idleReviewer.)
func reviewerAssignable(a store.Agent) bool {
	return a.Role == "reviewer" && !a.ClearArmed
}

// freeReviewer returns a running reviewer holding no review, checking the project's own roster
// first, then GlobalProject's — a submit reaches the pool here too, not just the periodic sweep.
func (e *Engine) freeReviewer(project string) (string, error) {
	if name, err := e.freeReviewerOn(project); name != "" || err != nil {
		return name, err
	}
	if project == GlobalProject {
		return "", nil
	}
	return e.freeReviewerOn(GlobalProject)
}

// freeReviewerOn is freeReviewer narrowed to one project's own roster.
func (e *Engine) freeReviewerOn(home string) (string, error) {
	roster, err := e.store.For(home).Roster()
	if err != nil {
		return "", fmt.Errorf("load roster for %s: %w", home, err)
	}
	for _, a := range roster {
		// The watchdog's standing observation, not a probe of our own — this runs off
		// RepairReviewRows' tick, once per open PR, and a fresh exec per row is what saturated
		// the runtime the observer now exists to prevent (-> hub/watchdog.go).
		if !reviewerAssignable(a) || !e.deps.AgentUp(home, a.Name) {
			continue
		}
		_, held, err := e.store.ReviewingPR(home, a.Name)
		if err != nil {
			return "", err
		}
		if held == "" {
			return a.Name, nil
		}
	}
	return "", nil
}
