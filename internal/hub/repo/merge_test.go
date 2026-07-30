package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// mergeRepo builds a repo whose branch "work" changes a file, with base ("main") checked out.
func mergeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	write := func(rel, body string) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); e != nil {
			t.Fatal(e)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	write("kept.txt", "base\n")
	run("add", "-A")
	run("commit", "-qm", "base")

	run("checkout", "-q", "-b", "work")
	write("kept.txt", "from the branch\n")
	write("added.txt", "the branch adds this\n")
	run("add", "-A")
	run("commit", "-qm", "branch work")
	run("checkout", "-q", "main")
	return root
}

// TestMergeBlockedByLocalState covers the three ways the user's own checkout stops a merge. Each
// must come back as MergeBlocked with the files to clear — a fixable situation the hub reports, not
// a MergeErr that reads as the hub breaking. The untracked case used to fall through to MergeErr
// because git words it differently from the tracked one.
func TestMergeBlockedByLocalState(t *testing.T) {
	t.Run("tracked edit in the way", func(t *testing.T) {
		root := mergeRepo(t)
		if err := os.WriteFile(filepath.Join(root, "kept.txt"), []byte("my uncommitted edit\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		res := MergeBranch(root, "", "work", "main")
		if res.Status != MergeBlocked {
			t.Fatalf("status = %q (err %v), want %q", res.Status, res.Err, MergeBlocked)
		}
		if len(res.Files) == 0 {
			t.Error("MergeBlocked must name the blocking files — clearing them is the whole instruction")
		}
	})

	t.Run("untracked file in the way", func(t *testing.T) {
		root := mergeRepo(t)
		// The branch adds added.txt; an untracked file of the same name blocks the merge.
		if err := os.WriteFile(filepath.Join(root, "added.txt"), []byte("mine, never committed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		res := MergeBranch(root, "", "work", "main")
		if res.Status != MergeBlocked {
			t.Fatalf("status = %q (err %v), want %q — git words untracked collisions differently", res.Status, res.Err, MergeBlocked)
		}
	})

	t.Run("clean checkout merges", func(t *testing.T) {
		root := mergeRepo(t)
		if res := MergeBranch(root, "", "work", "main"); res.Status != MergeDone {
			t.Fatalf("status = %q (err %v), want %q", res.Status, res.Err, MergeDone)
		}
	})
}

// TestMergeBlockedUnderATranslatedLocale is why git.Merge pins the locale: the block is recognised
// by git's own wording, so a user working in another language would otherwise get an opaque failure
// where an English-locale user gets the files to fix. Runs the same case with the environment set
// to German; git may not have that translation installed, in which case pinning is moot and the
// test still passes — it can only fail if the pin is REMOVED and a translation is present.
func TestMergeBlockedUnderATranslatedLocale(t *testing.T) {
	for _, kv := range [][2]string{{"LC_ALL", "de_DE.UTF-8"}, {"LANGUAGE", "de"}, {"LANG", "de_DE.UTF-8"}} {
		t.Setenv(kv[0], kv[1])
	}
	root := mergeRepo(t)
	if err := os.WriteFile(filepath.Join(root, "kept.txt"), []byte("my uncommitted edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := MergeBranch(root, "", "work", "main")
	if res.Status != MergeBlocked {
		t.Fatalf("status = %q (err %v), want %q — git's message must reach the matcher in English", res.Status, res.Err, MergeBlocked)
	}
}
