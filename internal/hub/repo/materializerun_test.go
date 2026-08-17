package repo

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMaterializeRunCopiesUncommittedAndSkipsGitAndCaches pins the artifact-isolation choice
// itself: a run's copy must include uncommitted files (the whole point of not using a git
// checkout) and must exclude .git and the directories a run's cache mount replaces anyway.
func TestMaterializeRunCopiesUncommittedAndSkipsGitAndCaches(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "agent-wt")
	mustMkdirAll(t, filepath.Join(src, ".git"))
	mustWriteFile(t, filepath.Join(src, ".git", "HEAD"), "ref: refs/heads/main\n")
	mustWriteFile(t, filepath.Join(src, "committed.go"), "package x\n")
	mustWriteFile(t, filepath.Join(src, "uncommitted.txt"), "not yet added\n")
	mustMkdirAll(t, filepath.Join(src, "sub"))
	mustWriteFile(t, filepath.Join(src, "sub", "nested.txt"), "nested\n")
	mustMkdirAll(t, filepath.Join(src, "node_modules", "leftpad"))
	mustWriteFile(t, filepath.Join(src, "node_modules", "leftpad", "index.js"), "module.exports = {}\n")
	mustMkdirAll(t, filepath.Join(src, "target", "debug"))
	mustWriteFile(t, filepath.Join(src, "target", "debug", "bin"), "binary\n")

	dst, err := MaterializeRun(root, src, "abc123")
	if err != nil {
		t.Fatalf("MaterializeRun: %v", err)
	}
	if want := filepath.Join(root, ".worktrees", "run-abc123"); dst != want {
		t.Errorf("dst = %q, want %q", dst, want)
	}
	mustExist(t, filepath.Join(dst, "committed.go"))
	mustExist(t, filepath.Join(dst, "uncommitted.txt"))
	mustExist(t, filepath.Join(dst, "sub", "nested.txt"))
	mustNotExist(t, filepath.Join(dst, ".git"))
	mustNotExist(t, filepath.Join(dst, "node_modules"))
	mustNotExist(t, filepath.Join(dst, "target"))

	if err := RemoveRunMaterialization(dst); err != nil {
		t.Fatalf("RemoveRunMaterialization: %v", err)
	}
	mustNotExist(t, dst)
}

// TestMaterializeRunClearsAStalePreviousCopy: a crash between runs must not leave a run seeing a
// mix of two attempts.
func TestMaterializeRunClearsAStalePreviousCopy(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "agent-wt")
	mustMkdirAll(t, src)
	mustWriteFile(t, filepath.Join(src, "fresh.txt"), "fresh\n")

	stale := filepath.Join(root, ".worktrees", "run-again")
	mustMkdirAll(t, stale)
	mustWriteFile(t, filepath.Join(stale, "leftover.txt"), "stale\n")

	dst, err := MaterializeRun(root, src, "again")
	if err != nil {
		t.Fatalf("MaterializeRun: %v", err)
	}
	mustExist(t, filepath.Join(dst, "fresh.txt"))
	mustNotExist(t, filepath.Join(dst, "leftover.txt"))
}

// TestMaterializeRunFromTheRepoRootSkipsWorktrees is the case a user run makes ordinary: with no
// agent named, the source is the repo ROOT, and every destination lives at root/.worktrees/run-<id>
// — so the destination is INSIDE the source. Unskipped, the walk copies every other agent's live
// worktree, with their uncommitted work in it, and then descends into the half-built destination
// and copies that into itself. Nothing bounds it: materialization runs before the container starts,
// so the run's 15-minute cap is not yet counting while the disk fills.
func TestMaterializeRunFromTheRepoRootSkipsWorktrees(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "main.go"), "package main\n")
	mustWriteFile(t, filepath.Join(root, "uncommitted.txt"), "what the user is working on\n")
	// Another agent's live worktree, and the shared review checkout, both under .worktrees.
	mustMkdirAll(t, filepath.Join(root, ".worktrees", "bombur"))
	mustWriteFile(t, filepath.Join(root, ".worktrees", "bombur", "theirs.go"), "package theirs\n")
	mustMkdirAll(t, filepath.Join(root, ".worktrees", "review"))
	mustWriteFile(t, filepath.Join(root, ".worktrees", "review", "pr.go"), "package pr\n")
	// And a copy left by an earlier run, which is what the walk would recurse into.
	mustMkdirAll(t, filepath.Join(root, ".worktrees", "run-earlier"))
	mustWriteFile(t, filepath.Join(root, ".worktrees", "run-earlier", "old.txt"), "old\n")

	dst, err := MaterializeRun(root, root, "self")
	if err != nil {
		t.Fatalf("MaterializeRun: %v", err)
	}
	// The user's own tree is the point of the target, uncommitted work included.
	mustExist(t, filepath.Join(dst, "main.go"))
	mustExist(t, filepath.Join(dst, "uncommitted.txt"))
	// Nobody else's, and not the destination itself.
	mustNotExist(t, filepath.Join(dst, ".worktrees"))
	// Nothing ANYWHERE beneath the copy, at any depth: the self-nesting shows up as a run- directory
	// inside the copy, and another agent's tree as a .worktrees inside it. Relative paths, since
	// the copy's own path is under .worktrees and would otherwise match itself.
	if err := filepath.WalkDir(dst, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dst, p)
		if err != nil || rel == "." {
			return err
		}
		if d.IsDir() && (d.Name() == ".worktrees" || strings.HasPrefix(d.Name(), "run-")) {
			t.Errorf("the copy swept in %s", rel)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk the copy: %v", err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s: want to exist, got %v", path, err)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s: want absent, but it exists", path)
	}
}
