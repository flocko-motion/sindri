// package: hub/core / repo
// type:    logic (the repo facts every subject reads)
// job:     the branch agents work against, and the once-per-root warning when nobody configured
// one. Held here because a task, a pull request and a run all need the same answer, and three
// copies of it would be three chances to disagree.
// limits:  reading. Branching, merging and rebasing are the callers' (-> adapter/git, adapter/git).
package core

import (
	"fmt"
	"os"
	"sync"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/config"
)

// refFallbackWarn remembers which repo roots have already been warned about the unconfigured-
// reference fallback below, so a call on every submit does not spam the log with the same finding.
type refFallbackWarn struct {
	mu   sync.Mutex
	seen map[string]bool
}

// BaseBranch is the branch agents work against: the configured `reference:`, else the main
// checkout's current branch. Configured-but-absent is fatal — every claim/submit/merge needs it.
func (c *Core) BaseBranch(root string) (string, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return "", err
	}
	if cfg.Reference == "" {
		branch, err := git.CurrentBranch(root)
		if err != nil {
			return "", err
		}
		c.warnUnconfiguredReference(root, branch)
		return branch, nil
	}
	if !git.BranchExists(root, cfg.Reference) {
		return "", fmt.Errorf("the configured reference branch %q doesn't exist in %s — create it or fix `reference:` in .sindri/config.yaml", cfg.Reference, root)
	}
	return cfg.Reference, nil
}

// warnUnconfiguredReference logs, once per root, that every agent's reference is whatever a human
// happens to have checked out in the main working copy — a fallback that moves silently the moment
// they switch branches there, with nothing on the board to say so.
func (c *Core) warnUnconfiguredReference(root, branch string) {
	c.refWarn.mu.Lock()
	defer c.refWarn.mu.Unlock()
	if c.refWarn.seen == nil { // a bare &Core{} in a test skips its own initialisation
		c.refWarn.seen = map[string]bool{}
	}
	if c.refWarn.seen[root] {
		return
	}
	c.refWarn.seen[root] = true
	fmt.Fprintf(os.Stderr, "hub: %s has no `reference:` configured — every agent measures against %q, whatever is checked out there right now; set `reference:` in .sindri/config.yaml to pin it.\n", root, branch)
}
