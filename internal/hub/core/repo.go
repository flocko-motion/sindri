// package: hub/core / repo
// type:    logic (the repo facts every subject reads)
// job:     hand every subject the branch agents work against. The resolution itself is
// world/reference's, so the board reports the same answer this hands a claim.
// limits:  reading. Branching, merging and rebasing are the callers' (-> adapter/git, adapter/git).
package core

import "github.com/flo-at/sindri/internal/hub/world/reference"

// BaseBranch is the branch agents work against: the configured `reference:`, else the main
// checkout's current branch. Both are settings, and an unpinned repo is not a misconfigured one —
// the branch a human has checked out IS the reference, and the Repos pane names which it is.
func (c *Core) BaseBranch(root string) (string, error) {
	branch, _, err := reference.Resolve(root)
	return branch, err
}
