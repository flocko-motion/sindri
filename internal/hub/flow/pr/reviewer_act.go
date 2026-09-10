// package: hub/flow/pr / reviewer_act
// type:    logic (what a reviewer is told, and the claim behind it)
// job:     answer a reviewer asking where it stands — the ONE pull request it holds, or the next
// unclaimed one taken up on the spot.
// limits:  the reviewer's side. Assigning a review to somebody is review_act.go's, and a pull request's
// own life is hub/flow/pr's.
package pr

import (
	"context"
	"fmt"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"os"

	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// ReviewDirective is what a reviewer is told: the ONE PR it holds, whose branch sits in its one
// workspace — serving a second while the first was still checked out left neither diff readable.
// Free, it claims the oldest unclaimed review, also how one with no reviewer running gets picked up.
func (a *Act) ReviewDirective(ctx context.Context, project, name string) (string, bool, error) {
	ps := a.Store.For(project)
	// heldProject, not project: a api.GlobalProject reviewer's held review is filed under whatever
	// project it was sent to, never its own.
	heldProject, held, err := a.Store.ReviewingPR(project, name)
	if err != nil {
		return "", false, err
	}
	if held != "" {
		hps := a.Store.For(heldProject)
		pr, ok, err := hps.GetPR(held)
		if err != nil {
			return "", false, err
		}
		if ok && pr.Status == "open" {
			return prompts.DirReview(pr.ID, pr.Task, a.TaskTitle(heldProject, pr.Task), pr.Agent, a.Deps.ArchitectureDoc(heldProject)), true, nil
		}
		// Settled while it was reading: a verdict on it now decides nothing, so the hold is released
		// rather than left to produce one.
		if err := hps.CloseReviews(held, "overtaken: the PR was "+pr.Status+" before a verdict"); err != nil {
			return "", false, err
		}
		_ = ps.SetState(store.AgentState{Agent: name, Phase: core.RestPhase("reviewer")},
			store.ReasonFreed, "review overtaken: "+held+" was "+pr.Status+" before a verdict")
		_ = a.Harness.Say(project, name, prompts.MsgReviewCancelled(held), mail.MailAndPush)
	}
	if a.Retired(project, name) {
		return prompts.DirRetired, true, nil // holds nothing now — retirement means no new claim, reviewer too
	}
	var id int64
	var prID string
	found, err := ps.UnclaimedReview(&id, &prID)
	if err != nil {
		return "", false, err
	}
	if !found {
		// `sindri` answers at once: AssignPendingReviews pushes a wake once a review is claimable.
		return prompts.DirNoReviews, true, nil
	}
	// An armed clear fires eagerly before any claim, same as fireClearIfArmed — and answers with the
	// clearing state for the same reason: the session that asked has just been discarded, so the
	// review is claimed on the ask that follows the kickoff rather than served into a reply nobody
	// reads.
	if a.ClearArmedFor(project, name) {
		if err := a.Harness.Clear(ctx, project, name); err != nil {
			return "", false, err
		}
		if err := a.Harness.Say(project, name, prompts.MsgKickoff, mail.PushOnly); err != nil {
			return "", false, err
		}
		return prompts.DirBusy("reviewer/clearing"), true, nil
	}
	// Claim FIRST — same reason ClaimNext claims before it prepares: AssignReview holds the review
	// before it clears, so a session discarded mid-way cannot lose the claim with it.
	req, _ := a.ReviewPrompt(project)
	if err := a.AssignReview(ctx, project, id, prID, name, req); err != nil {
		return "", false, err
	}
	pr, _, _ := ps.GetPR(prID)
	return prompts.DirReview(prID, pr.Task, a.TaskTitle(project, pr.Task), pr.Agent, a.Deps.ArchitectureDoc(project)), true, nil
}

// ReleaseReviewers closes every open review of a PR and frees whoever held one, telling them the PR
// is settled. Called wherever a PR reaches a terminal state, so no reviewer is left holding a
// verdict that can no longer mean anything.
func (a *Act) ReleaseReviewers(project, prID, why string) {
	ps := a.Store.For(project)
	revs, _ := ps.Reviews(prID)
	if err := ps.CloseReviews(prID, why); err != nil {
		fmt.Fprintf(os.Stderr, "hub: closing reviews of %s: %v\n", prID, err)
		return
	}
	for _, r := range revs {
		if r.Author == "" || r.Verdict != "" {
			continue
		}
		_ = ps.SetState(store.AgentState{Agent: r.Author, Phase: core.RestPhase("reviewer")}, store.ReasonFreed, why)
		_ = a.Harness.Say(project, r.Author, prompts.MsgReviewCancelled(prID), mail.MailAndPush)
	}
	a.Deps.Notify()
}

// ReviewerHolding returns the open review of prID and who is doing it, or (0, "") if nobody is.
func (a *Act) ReviewerHolding(project, prID string) (int64, string) {
	revs, err := a.Store.For(project).Reviews(prID)
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

// ReviewerAssignable reports whether an agent may be handed a review. The refusal is the surface's:
// an armed clear disqualifies one because PR after PR would defer it for ever, and an assignment
// slipping in while the tick fires the clear would clear a reviewer mid-review.
func ReviewerAssignable(s situation.Situation) bool {
	return s.Role == "reviewer" && situation.Allows(s.Allowed().Assign)
}

// freeReviewer returns a running reviewer holding no review, checking the project's own roster
// first, then api.GlobalProject's — a submit reaches the pool here too, not just the periodic sweep.
func (a *Act) freeReviewer(project string) (string, error) {
	if name, err := a.freeReviewerOn(project); name != "" || err != nil {
		return name, err
	}
	if project == api.GlobalProject {
		return "", nil
	}
	return a.freeReviewerOn(api.GlobalProject)
}

// freeReviewerOn is freeReviewer narrowed to one project's own roster. One gather for the whole
// roster, so the claimable pool behind those situations is read once rather than once per candidate.
func (a *Act) freeReviewerOn(home string) (string, error) {
	roster, err := a.Sit.Roster(home)
	if err != nil {
		return "", fmt.Errorf("load roster for %s: %w", home, err)
	}
	for _, s := range roster {
		// The watchdog's standing observation, not a probe of our own — this runs off
		// RepairReviewRows' tick, once per open PR, and a fresh exec per row is what saturated
		// the runtime the observer now exists to prevent (-> hub/watchdog.go).
		if ReviewerAssignable(s) && s.Up && s.ReviewingPR == "" {
			return s.Name, nil
		}
	}
	return "", nil
}
