// package: hub/flow/pr / conflict_act
// type:    logic (whether a conflict stands against a merge intent)
// job:     read a pull request's own record for an unanswered conflict, and record one. The single
// account of it: a conflict is a fact about the merge intent, and what it means for the author is
// its own map's (-> cond.MergeConflicted).
// limits:  the record. Producing a conflict belongs to whoever ran the rebase.
package pr

import (
	"strings"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// ConflictStanding reports a conflict recorded against this merge intent and not yet answered. Read
// from the record rather than returned by whoever hit it, so a conflict whose reply was lost still
// reaches the author — and so the author's map and the verb it types cannot disagree about one.
func ConflictStanding(ps *store.ProjectStore, prID string) bool {
	if prID == "" {
		return false
	}
	events, err := ps.PREvents(prID)
	if err != nil {
		return false
	}
	for i := len(events) - 1; i >= 0; i-- {
		switch events[i].Type {
		case "conflict":
			return true
		case "merged", "scrapped", "resubmitted", "renewed":
			return false // superseded: the conflict was answered
		}
	}
	return false
}

// RecordConflict writes one against a merge intent that exists — the fact the author's map reads to
// put it on the resolution. An id naming no pull request records nothing: the agent is told what to
// fix in the same breath, and an event under a name nobody has would answer to nothing.
func (a *Act) RecordConflict(ps *store.ProjectStore, prID, onto string, files []string) {
	if _, ok, err := ps.GetPR(prID); err != nil || !ok {
		return
	}
	_ = ps.LogPR(prID, "conflict", "rebase onto "+onto+" conflicts: "+strings.Join(files, ", "))
}
