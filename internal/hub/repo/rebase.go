// package: hub/repo / rebase
// type:    logic (git rebase mechanics)
// job:     the worker-driven incremental rebase step — advance an in-progress rebase
// or start a fresh one of a branch onto its base — reporting conflicts,
// completion, or a git error so the workflow can loop the worker through
// conflict resolution.
// limits:  git only; the policy (dirty-tree guard, phase transitions, the messages
// shown to the worker) is the workflow's.
package repo

import "github.com/flo-at/sindri/internal/adapter/git"

// RebaseStep advances an in-progress rebase, finishes a stranded autostash conflict, or starts a
// fresh one — the single step the worker-driven rebase loops repeat until done. The stranded case
// goes first: a fresh start opens with a checkout, which git refuses while the index is unmerged.
func RebaseStep(wt, branch, base string) (conflicts []string, done bool, err error) {
	switch {
	case git.RebaseInProgress(wt):
		return git.RebaseContinue(wt)
	case git.StashConflict(wt):
		return git.ResolveStashConflict(wt)
	}
	return git.RebaseStart(wt, branch, base)
}
