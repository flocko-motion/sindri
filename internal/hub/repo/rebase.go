// package: hub/repo / rebase
// type:    logic (git rebase mechanics)
// job:     the worker-driven incremental rebase step — advance an in-progress rebase
//          or start a fresh one of a branch onto its base — reporting conflicts,
//          completion, or a git error so the workflow can loop the worker through
//          conflict resolution.
// limits:  git only; the policy (dirty-tree guard, phase transitions, the messages
//          shown to the worker) is the workflow's.
package repo

import "github.com/flo-at/sindri/internal/adapter/git"

// RebaseStep advances an in-progress rebase in wt, or starts a fresh rebase of branch
// onto base when none is underway. It returns the conflicting files (empty when it
// applied cleanly), done=true once the branch is fully rebased, and any git error —
// the single step the worker-driven resolve/rebase loops repeat until done.
func RebaseStep(wt, branch, base string) (conflicts []string, done bool, err error) {
	if git.RebaseInProgress(wt) {
		return git.RebaseContinue(wt)
	}
	return git.RebaseStart(wt, branch, base)
}
