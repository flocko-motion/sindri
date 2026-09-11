// package: hub/flow/pr / reviewer_act
// type:    logic (who holds a review, and letting go of one)
// job:     release the reviewers a settled pull request no longer needs, and say who is holding one.
// limits:  the reviewer's side. Assigning a review is review_act.go's, and a pull request's own life
// is hub/flow/pr's.
package pr

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// ReleaseReviewers closes every open review of a PR so no reviewer is left holding a verdict that
// can no longer mean anything. It is the ONE place a review is closed, which is what stops a second
// path releasing a reviewer somewhere the machine cannot see.
//
// The reviewer itself is not touched: it stands in reviewing, its own map observes the hold it no
// longer has, and its dropping state releases it and tells it so. Writing the reviewer from here was
// one function moving somebody else's subject, and the two accounts drifted.
func (a *Act) ReleaseReviewers(project, prID, why string) {
	if err := a.Store.For(project).CloseReviews(prID, why); err != nil {
		fmt.Fprintf(os.Stderr, "hub: closing reviews of %s: %v\n", prID, err)
		return
	}
	a.Flow.WakeProject(project, topic.PRVerdict)
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
