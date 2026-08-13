// package: hub/workflow / reviewhealth
// type:    logic (the review-row invariant)
// job:     every open, non-interim PR should carry a live review row; find and repair the
// ones that don't, through RequestReview (-> hub/refwatch.go for the tick).
// limits:  the invariant and its repair only.
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
