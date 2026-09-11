// package: hub/flow/pr / reviewhealth_act
// type:    logic (the review row a filed pull request needs)
// job:     open the live review row that makes a pull request claimable, which is what pr/requesting
// runs. The ordinary path and the repair are one thing: a pull request whose row was lost stands
// unreviewed exactly as a freshly filed one does.
// limits:  the row. WHO takes it is the reviewer's own map (-> roles/reviewer's taking).
package pr

import (
	"context"

	"github.com/flo-at/sindri/internal/hub/flow/topic"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// NeedsReview reports whether p is open, wants a review (never an interim PR — a mid-task
// contribution or a milestone, both waiting on the human), and has no live row covering it.
func NeedsReview(p store.PR, live map[string]bool) bool {
	return p.Status == "open" && p.Kind != "interim" && !live[p.ID]
}

// OpenReviewRow is what pr/requesting does: file the row that makes this pull request claimable. It
// hands the row to nobody — a reviewer takes it through its own map, from a session that has been
// cleared for it, which is what stops two paths preparing the same reviewer twice.
func (a *Act) OpenReviewRow(_ context.Context, w World) (Outcome, error) {
	ps := a.Store.For(w.Project)
	if w.ReviewFiled || w.Interim {
		return Nothing, nil
	}
	requirement, _ := a.ReviewPrompt(w.Project)
	if _, err := ps.AddReview(w.ID, requirement); err != nil {
		return Nothing, err
	}
	_ = ps.LogPR(w.ID, "review-requested", "unassigned — the next free reviewer takes it")
	a.Deps.Notify()
	// Every reviewer in the project, since any free one could be the one to take it — and a reviewer
	// whose pod was reclaimed is started by its own map on the strength of this row.
	a.Flow.WakeProject(w.Project, topic.PRVerdict)
	return Opened, nil
}
