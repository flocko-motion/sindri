// package: hub/repo / materializerun
// type:    logic (run workspace materialization)
// job:     copy an agent's live worktree into a fresh, disposable directory for a run
// to execute against — including uncommitted changes, which a `git worktree
// add` checkout (-> MaterializeReview) never sees.
// limits:  a plain recursive file copy; the container mount, cache, and cleanup
// timing are workflow's (-> workflow/execrun.go).
package repo

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// runSkipDirs skip .git (no history needed) and node_modules/target (the run's cache mount replaces
// them -> workflow/execrun.go). .worktrees is load-bearing rather than tidiness: a run against the
// repo ROOT has its destination INSIDE its source, so without it the walk sweeps in every other
// agent's live worktree and then recurses into its own half-built copy — before the container
// starts, so the run's cap is not yet counting.
var runSkipDirs = map[string]bool{".git": true, "node_modules": true, "target": true, ".worktrees": true}

// MaterializeRun copies srcWorktree into root/.worktrees/run-<runID>, fresh each time — the
// artifact-isolation choice for sd-938f23: a run executes against this COPY, never the agent's
// live worktree, so git.CommitAll's `git add -A` can never sweep test exhaust into a PR.
func MaterializeRun(root, srcWorktree, runID string) (string, error) {
	dst := filepath.Join(root, ".worktrees", "run-"+runID)
	if err := os.RemoveAll(dst); err != nil {
		return "", fmt.Errorf("clear run workspace: %w", err)
	}
	if err := copyTree(srcWorktree, dst); err != nil {
		_ = os.RemoveAll(dst)
		return "", fmt.Errorf("materialize run workspace: %w", err)
	}
	return dst, nil
}

// RemoveRunMaterialization discards a run's materialized copy — best-effort, since it runs
// after the run already has its result.
func RemoveRunMaterialization(path string) error {
	return os.RemoveAll(path)
}

// copyTree recursively copies src to dst, skipping runSkipDirs and anything not a regular file.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if d.IsDir() && runSkipDirs[d.Name()] {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(p, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
