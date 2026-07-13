// package: hub/repo / merge
// type:    logic (git merge mechanics)
// job:     the rebase-first merge of a PR branch into its base, as a pure mechanic:
//          rebase the branch onto base (in the agent's worktree) then merge it into
//          base, classifying the outcome (done / conflict / blocked / error) so the
//          workflow orchestrator can route the consequences.
// limits:  git only — no store, no messaging, no status writes. It reports what
//          happened; the workflow decides what to do about it.
package repo

import (
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
)

// MergeStatus classifies the outcome of a MergeBranch attempt.
type MergeStatus string

const (
	// MergeDone: the branch rebased cleanly and merged into base.
	MergeDone MergeStatus = "done"
	// MergeConflict: rebasing onto base hit conflicts (Files lists them) — the worker
	// must resolve them; the branch is not merged.
	MergeConflict MergeStatus = "conflict"
	// MergeRebaseErr: the rebase could not run at all (Err set) — usually uncommitted
	// changes in the agent's worktree.
	MergeRebaseErr MergeStatus = "rebase-error"
	// MergeBlocked: the merge is blocked by uncommitted local changes in the main
	// checkout (Files lists them) — the branch is fine, just retry after clearing them.
	MergeBlocked MergeStatus = "blocked-local"
	// MergeErr: the merge itself failed for some other reason (Err set).
	MergeErr MergeStatus = "merge-error"
)

// MergeResult is the outcome of MergeBranch: a status plus whichever detail that
// status carries (conflicting/blocking files, or the underlying git error).
type MergeResult struct {
	Status MergeStatus
	Files  []string
	Err    error
}

// MergeBranch brings branch up to base then merges it into base in root, reporting
// the outcome without touching any state. When worktree is non-empty the branch is
// rebased onto base there first (a stale branch rebases silently; a conflict stops
// with MergeConflict); an empty worktree skips the rebase (e.g. a planner PR with no
// agent worktree). The workflow interprets the result and drives the consequences.
func MergeBranch(root, worktree, branch, base string) MergeResult {
	if worktree != "" {
		conflicts, done, err := git.RebaseStart(worktree, branch, base)
		if err != nil {
			return MergeResult{Status: MergeRebaseErr, Err: err}
		}
		if !done {
			return MergeResult{Status: MergeConflict, Files: conflicts}
		}
	}
	if err := git.Merge(root, base, branch); err != nil {
		if e := err.Error(); strings.Contains(e, "would be overwritten") || strings.Contains(e, "commit your changes or stash") {
			return MergeResult{Status: MergeBlocked, Files: git.BlockingLocalChanges(root, base, branch)}
		}
		return MergeResult{Status: MergeErr, Err: err}
	}
	return MergeResult{Status: MergeDone}
}
