package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// refRepo builds a repo on branch "side" with "main" beside it, so a pinned reference and an
// inferred one give different answers and a test can tell which one the state reports.
func refRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "base")
	run("checkout", "-q", "-b", "side")
	return root
}

// writeConfig puts body in the repo's .sindri/config.yaml.
func writeConfig(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".sindri")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDocStateReportsTheReferenceBranch: the board carries the branch agents work against, so the
// Repos tab renders it instead of the user never learning the key exists. Pinned must be told from
// inferred — both are working repos, and only the flag says which one a reader is looking at.
func TestDocStateReportsTheReferenceBranch(t *testing.T) {
	s := &Service{}
	root := refRepo(t) // checked out on "side"

	st := s.DocState(root)
	if st.Reference != "side" || st.ReferencePinned {
		t.Errorf("unset should report the checkout's branch unpinned, got %q pinned=%v", st.Reference, st.ReferencePinned)
	}
	if st.ReferenceAdvice != "" {
		t.Errorf("following the checkout is the design, not a fault: %q", st.ReferenceAdvice)
	}

	writeConfig(t, root, "reference: main\n")
	if st = s.DocState(root); st.Reference != "main" || !st.ReferencePinned {
		t.Errorf("a pinned reference should win and read as pinned, got %q pinned=%v", st.Reference, st.ReferencePinned)
	}
}

// TestDocStateFaultsWithNoBranchToWorkFrom: a detached main checkout leaves the project with no
// reference at all, which jams every claim — the one reference situation that IS a fault.
func TestDocStateFaultsWithNoBranchToWorkFrom(t *testing.T) {
	s := &Service{}
	root := refRepo(t)
	if out, err := exec.Command("git", "-C", root, "checkout", "-q", "--detach").CombinedOutput(); err != nil {
		t.Fatalf("detach: %s", out)
	}

	st := s.DocState(root)
	if st.Reference != "" || st.ReferencePinned {
		t.Errorf("a detached checkout names no branch, got %q pinned=%v", st.Reference, st.ReferencePinned)
	}
	if !strings.Contains(st.ReferenceAdvice, "no reference branch to work from") {
		t.Errorf("the advice must say what is missing, got %q", st.ReferenceAdvice)
	}

	// A `reference:` naming a branch nobody created is the other fault, and stays flagged as pinned
	// so a reader is not told to check out a branch when the config is what needs fixing.
	writeConfig(t, root, "reference: does-not-exist\n")
	if st = s.DocState(root); !st.ReferencePinned || st.ReferenceAdvice == "" {
		t.Errorf("a missing pinned branch must report a pinned fault, got pinned=%v advice %q", st.ReferencePinned, st.ReferenceAdvice)
	}
}
