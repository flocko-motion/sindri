package git

import (
	"os"
	"path/filepath"
	"testing"
)

// TestArchiveTreeExportsPlainFilesNoGit is the reviewer-has-no-git-of-its-own case: a global
// reviewer's fixed workspace gets a ref's files, not a checkout, and never carries a stale file
// from whichever PR the last review left there.
func TestArchiveTreeExportsPlainFilesNoGit(t *testing.T) {
	repo := newRepo(t)
	gitOut(t, repo, "checkout", "-q", "-b", "feat")
	mustWrite(t, repo, "new.txt", "feat content\n")
	gitOut(t, repo, "add", "-A")
	gitOut(t, repo, "commit", "-qm", "feat commit")

	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "stale.txt"), []byte("leftover"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ArchiveTree(repo, "feat", dest); err != nil {
		t.Fatalf("ArchiveTree: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "new.txt"))
	if err != nil {
		t.Fatalf("new.txt missing: %v", err)
	}
	if string(got) != "feat content\n" {
		t.Errorf("new.txt = %q, want feat's content", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "f")); err != nil {
		t.Errorf("f from the base commit should also be present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "stale.txt")); !os.IsNotExist(err) {
		t.Errorf("stale.txt from the last review should have been cleared, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); !os.IsNotExist(err) {
		t.Error("no .git should be exported — a plain tree, not a repository")
	}
}

// TestArchiveTreeCreatesAnAbsentDest: a reviewer's first assignment finds no workspace directory
// yet, so materialising must not require one to already exist.
func TestArchiveTreeCreatesAnAbsentDest(t *testing.T) {
	repo := newRepo(t)
	dest := filepath.Join(t.TempDir(), "not-yet-created")

	if err := ArchiveTree(repo, "HEAD", dest); err != nil {
		t.Fatalf("ArchiveTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "f")); err != nil {
		t.Errorf("f should be exported into the newly created dest: %v", err)
	}
}
