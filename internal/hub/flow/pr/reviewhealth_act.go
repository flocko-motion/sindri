// package: hub/flow/pr / reviewhealth_act
// type:    logic (the review-row invariants)
// job:     two things that must hold of every open, non-interim PR: it carries a live review
// row, and no live row sits unclaimed while a reviewer is free to take it. Find and
// repair both (-> hub/refwatch.go for the tick).
// limits:  the invariants and their repair only.
package pr

import (
	"fmt"
	"github.com/flo-at/sindri/internal/api"
	"os"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// NeedsReview reports whether p is open, wants a review (never an interim PR — a mid-task
// contribution or a milestone, both waiting on the human), and has no live row covering it.
func NeedsReview(p store.PR, live map[string]bool) bool {
	return p.Status == "open" && p.Kind != "interim" && !live[p.ID]
}

// RepairReviewRows requests a review for every open, non-interim PR in project missing a live row.
func (a *Act) RepairReviewRows(project string) {
	ps := a.Store.For(project)
	prs, err := ps.PRs("open")
	if err != nil {
		return
	}
	live, err := ps.LiveReviewPRs()
	if err != nil {
		return
	}
	for _, p := range prs {
		if !NeedsReview(p, live) {
			continue
		}
		if err := a.RequestReview(project, p.ID, ""); err != nil {
			fmt.Fprintf(os.Stderr, "hub: repair review row for %s: %v\n", p.ID, err)
			continue
		}
		_ = ps.LogPR(p.ID, "review-repaired", "no live review row was found; requested one")
	}
}

// AssignPendingReviews looks at the reviewers unclaimed rows are waiting on, so one that stopped
// asking is not left idle beside a PR. The reviewer's own map takes it from there — the one route
// in, so a pushed review is prepared for exactly as an asked-for one is (-> reviewer/clearing).
func (a *Act) AssignPendingReviews(project string) {
	ps := a.Store.For(project)
	// Each hand-over spends a reviewer, so the loop drains as many rows as there are idle ones. It
	// stops the moment a look leaves the row unclaimed, which is what keeps it from spinning.
	for {
		var id int64
		var prID string
		found, err := ps.UnclaimedReview(&id, &prID)
		if err != nil || !found {
			return
		}
		reviewer, err := a.IdleReviewer(project)
		if err != nil {
			return
		}
		if reviewer == "" {
			// Nobody up: the hub reclaims an idle reviewer's pod after 30 minutes, and dvalin's PR sat
			// unassigned while both were stopped. Waking one is the missing half of reclaiming it.
			a.wakeAReviewer(project)
			return
		}
		home, _, _ := a.reviewerHome(project, reviewer)
		a.Flow.Look(home, reviewer)
		if _, held, _ := a.Store.ReviewingPR(home, reviewer); held != prID {
			return // its map did not take this row; a second pass would ask the same question
		}
		// Logged on the agent too: a review that arrived because nobody came for it is a repair.
		_ = ps.Log(reviewer, "review-pushed", prID+" (unclaimed; the hub woke the reviewer for it)")
	}
}

// IdleReviewer returns a reviewer holding no review and sitting at an idle prompt: the project's own
// roster first, then the fleet-wide pool. Liveness is the watchdog's standing observation, so "is it
// up" and "can it be told anything" are one question from one moment.
func (a *Act) IdleReviewer(project string) (string, error) {
	if name, err := a.idleReviewerOn(project); name != "" || err != nil {
		return name, err
	}
	if project == api.GlobalProject {
		return "", nil
	}
	return a.idleReviewerOn(api.GlobalProject)
}

// idleReviewerOn is IdleReviewer narrowed to one project's own roster. An idle PROMPT is the extra
// condition here, since this answers "who could take it right now" rather than "who may be given it".
func (a *Act) idleReviewerOn(home string) (string, error) {
	roster, err := a.Sit.Roster(home)
	if err != nil {
		return "", fmt.Errorf("load roster for %s: %w", home, err)
	}
	for _, s := range roster {
		if ReviewerAssignable(s) && s.ReviewingPR == "" && s.AtPrompt() {
			return s.Name, nil
		}
	}
	return "", nil
}

// wakeAReviewer starts a stopped reviewer, so work arriving for an empty pool brings one back. One
// per call: the next tick wakes a second only if a second row is still waiting.
func (a *Act) wakeAReviewer(project string) {
	for _, home := range []string{project, api.GlobalProject} {
		roster, err := a.Sit.Roster(home)
		if err != nil {
			continue
		}
		for _, s := range roster {
			// Stopped, never down: a pod the hub reclaimed comes back on its own session, where one
			// that died did so for a reason a restart here would just repeat.
			if !ReviewerAssignable(s) || !s.Stopped || s.Up {
				continue
			}
			if err := a.Harness.Start(home, s.Name); err != nil {
				fmt.Fprintf(os.Stderr, "hub: waking %s for a waiting review: %v\n", s.Name, err)
				continue
			}
			_ = a.Store.For(home).Log(s.Name, "woken", "a review is waiting and no reviewer was up")
			return
		}
		if project == api.GlobalProject {
			return
		}
	}
}
