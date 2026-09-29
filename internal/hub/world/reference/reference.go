// package: hub/world/reference / reference
// type:    logic (the branch agents work against)
// job:     resolve a repo's reference branch — the configured `reference:`, else the branch its
// main checkout is on — so the subject that ACTS on it and the surface that REPORTS it read one
// rule rather than two that can disagree.
// limits:  reading. Warning about the answer and acting on it are the callers' (-> hub/core).
package reference

import (
	"fmt"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/config"
)

// Resolve is the branch agents branch from and merge into: the configured `reference:`, else the
// branch the main checkout is on. Pinned says which of the two answered.
func Resolve(root string) (branch string, pinned bool, err error) {
	cfg, err := config.Load(root)
	if err != nil {
		return "", false, err
	}
	if cfg.Reference == "" {
		branch, err := git.CurrentBranch(root)
		if err != nil {
			// The checked-out branch IS the reference, so a checkout on none — a tag, a bare commit —
			// leaves the project with nothing to claim, submit or merge against.
			return "", false, fmt.Errorf("%w, so there is no reference branch to work from — check one out", err)
		}
		return branch, false, nil
	}
	if !git.BranchExists(root, cfg.Reference) {
		return "", true, fmt.Errorf("the configured reference branch %q doesn't exist in %s — create it or fix `reference:` in .sindri/config.yaml", cfg.Reference, root)
	}
	return cfg.Reference, true, nil
}
