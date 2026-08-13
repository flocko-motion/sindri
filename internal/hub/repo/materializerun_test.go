package repo

import (
	"os"
	"path/filepath"
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
