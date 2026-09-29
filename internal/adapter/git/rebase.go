// package: adapter/git / rebase
// type:    logic (git rebase mechanics)
// job:     the worker-driven incremental rebase step — advance an in-progress rebase
// or start a fresh one of a branch onto its base — reporting conflicts,
// completion, or a git error so the workflow can loop the worker through
// conflict resolution.
// limits:  git only; the policy (dirty-tree guard, phase transitions, the messages
// shown to the worker) is hub/flow/pr's.
package git

// RebaseStep advances an in-progress rebase, finishes a stranded autostash conflict, or starts a
// fresh one — the single step the worker-driven rebase loops repeat until done. The stranded case
// goes first: a fresh start opens with a checkout, which git refuses while the index is unmerged.
func RebaseStep(wt, branch, base string) (conflicts []string, done bool, err error) {
	switch {
	case RebaseInProgress(wt):
		return RebaseContinue(wt)
	case StashConflict(wt):
		return ResolveStashConflict(wt)
	}
	return RebaseStart(wt, branch, base)
}
