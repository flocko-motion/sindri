// package: hub/workflow / reviewhealth
// type:    logic (the review-row invariants)
// job:     two things that must hold of every open, non-interim PR: it carries a live review
// row, and no live row sits unclaimed while a reviewer is free to take it. Find and
// repair both (-> hub/refwatch.go for the tick).
// limits:  the invariants and their repair only.
package workflow

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/hub/store"
)

// needsReview reports whether p is open, wants a review (never an interim PR — a mid-task
// contribution or a milestone, both waiting on the human), and has no live row covering it.
func needsReview(p store.PR, live map[string]bool) bool {
	return p.Status == "open" && p.Kind != "interim" && !live[p.ID]
}

// RepairReviewRows requests a review for every open, non-interim PR in project missing a live row.
func (e *Engine) RepairReviewRows(project string) {
	ps := e.store.For(project)
	prs, err := ps.PRs("open")
	if err != nil {
		return
	}
	live, err := ps.LiveReviewPRs()
	if err != nil {
		return
	}
	for _, p := range prs {
		if !needsReview(p, live) {
			continue
		}
		if err := e.RequestReview(project, p.ID, ""); err != nil {
			fmt.Fprintf(os.Stderr, "hub: repair review row for %s: %v\n", p.ID, err)
			continue
		}
		_ = ps.LogPR(p.ID, "review-repaired", "no live review row was found; requested one")
	}
}

// AssignPendingReviews hands out the review rows nobody claimed. `sindri` answers a reviewer's own
// ask at once (-> reviewDirective), but one that stopped asking would otherwise sit idle beside a
// PR waiting on it until it happened to ask again — so the hub claims it here instead, periodically,
// for whichever reviewer is genuinely at an idle prompt to receive it.
func (e *Engine) AssignPendingReviews(project string) {
	ps := e.store.For(project)
	// Each assignment spends a reviewer, so the loop drains as many rows as there are idle ones.
	for {
		var id int64
		var prID string
		found, err := ps.UnclaimedReview(&id, &prID)
		if err != nil || !found {
			return
		}
		reviewer, err := e.idleReviewer(project)
		if err != nil || reviewer == "" {
			return
		}
		req, _ := e.ReviewPrompt(project)
		if err := e.assignReview(project, id, prID, reviewer, req); err != nil {
			fmt.Fprintf(os.Stderr, "hub: assigning %s to %s: %v\n", prID, reviewer, err)
			return
		}
		// Logged on the agent too: a review that arrived because nobody came for it is a repair.
		_ = ps.Log(reviewer, "review-pushed", prID+" (unclaimed; the hub handed it over)")
	}
}

// idleReviewer returns a reviewer holding no review and sitting at an idle prompt. Liveness is the
// watchdog's standing observation rather than freeReviewer's probe per call, so "is it up" and "can
// it be told anything" are one question from one moment. Retired is honoured as claimNext does.
func (e *Engine) idleReviewer(project string) (string, error) {
	ps := e.store.For(project)
	roster, err := ps.Roster()
	if err != nil {
		return "", fmt.Errorf("load roster for %s: %w", project, err)
	}
	for _, a := range roster {
		// ClearArmed for the same reason as Retired: a review handed over now would be cut in half
		// by the clear that is about to land.
		if a.Role != "reviewer" || a.Retired || a.ClearArmed || !e.deps.AgentIdle(project, a.Name) {
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
