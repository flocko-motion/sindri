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
		if err != nil {
			return
		}
		if reviewer == "" {
			// Nobody up. The hub reclaims an idle reviewer's pod after 30 minutes and nothing put one
			// back, so a PR submitted into an empty pool waited for a reviewer that would never come:
			// dvalin's sat unassigned while both were stopped. Waking one is the missing half of
			// reclaiming it — the next tick assigns the row once it is up.
			e.wakeAReviewer(project)
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

// idleReviewer returns a reviewer holding no review and sitting at an idle prompt: the project's own
// roster first, since a repo keeping its own reviewer expects it used, then GlobalProject's — a
// fleet-wide pool a project without one can still draw on. Liveness is the watchdog's standing
// observation rather than freeReviewer's probe per call, so "is it up" and "can it be told anything"
// are one question from one moment. Retired is honoured as claimNext does.
func (e *Engine) idleReviewer(project string) (string, error) {
	if name, err := e.idleReviewerOn(project); name != "" || err != nil {
		return name, err
	}
	if project == GlobalProject {
		return "", nil
	}
	return e.idleReviewerOn(GlobalProject)
}

// idleReviewerOn is idleReviewer narrowed to one project's own roster. An idle PROMPT is the extra
// condition here, since this answers "who could take it right now" rather than "who may be given it".
func (e *Engine) idleReviewerOn(home string) (string, error) {
	roster, err := e.sit.Roster(home)
	if err != nil {
		return "", fmt.Errorf("load roster for %s: %w", home, err)
	}
	for _, s := range roster {
		if reviewerAssignable(s) && s.ReviewingPR == "" && e.deps.AgentIdle(home, s.Name) {
			return s.Name, nil
		}
	}
	return "", nil
}

// wakeAReviewer starts a stopped reviewer, so work arriving for an empty pool brings one back. The
// project's own roster first, then the shared pool, matching who would be ASKED (-> idleReviewer).
// One per call: a second is woken by the next tick only if a second row is still waiting.
func (e *Engine) wakeAReviewer(project string) {
	for _, home := range []string{project, GlobalProject} {
		roster, err := e.sit.Roster(home)
		if err != nil {
			continue
		}
		for _, s := range roster {
			// Stopped, never down: a pod the hub reclaimed comes back on its own session, where one
			// that died did so for a reason a restart here would just repeat.
			if !reviewerAssignable(s) || !s.Stopped || s.Up {
				continue
			}
			if err := e.deps.StartAgent(home, s.Name); err != nil {
				fmt.Fprintf(os.Stderr, "hub: waking %s for a waiting review: %v\n", s.Name, err)
				continue
			}
			_ = e.store.For(home).Log(s.Name, "woken", "a review is waiting and no reviewer was up")
			return
		}
		if project == GlobalProject {
			return
		}
	}
}
