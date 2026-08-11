package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// prCheckEngine builds a repo whose base has moved past one open PR, and an engine over it.
func prCheckEngine(t *testing.T) (*Engine, *store.ProjectStore, string) {
	t.Helper()
	root := t.TempDir()
	if err := exec.Command("git", "init", "-q", "-b", "main", root).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run(root, "config", "user.email", "t@t")
	run(root, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(root, "add", "-A")
	run(root, "commit", "-q", "-m", "base")
	wt := filepath.Join(root, ".worktrees", "bombur")
	run(root, "worktree", "add", "-q", "-b", "sd-1", wt, "HEAD")

	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: ".worktrees/bombur"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-sd-1", Task: "sd-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "submitted"}); err != nil {
		t.Fatal(err)
	}
	return New(st, &stubDeps{root: root}), ps, root
}

// commitIn writes and commits a file, so a test can move either side.
func commitIn(t *testing.T, dir, name, body, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", msg}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
}

// prEventTypes is the finding trail recorded against a PR.
func prEventTypes(t *testing.T, ps *store.ProjectStore, id string) []string {
	t.Helper()
	evs, err := ps.PREvents(id)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range evs {
		out = append(out, e.Type+" "+e.Payload)
	}
	return out
}

// TestAPRLevelWithItsBaseIsNotChecked is tier 1 doing its job. The expensive check exists for a PR
// the base has moved past; running it on one that is level would spend a full gate run per sweep to
// re-answer a question nothing has changed.
func TestAPRLevelWithItsBaseIsNotChecked(t *testing.T) {
	e, ps, _ := prCheckEngine(t)
	e.CheckOpenPRs("proj")
	if got := prEventTypes(t, ps, "pr-sd-1"); len(got) != 0 {
		t.Errorf("a PR level with its base needs no check, got %v", got)
	}
}

// TestABehindPRIsCheckedAndTheFindingLandsOnThePR: the whole point. After the base moves, the PR
// carries the answer where the human reading it will see it.
func TestABehindPRIsCheckedAndTheFindingLandsOnThePR(t *testing.T) {
	e, ps, root := prCheckEngine(t)
	commitIn(t, root, "base-moved.txt", "later\n", "base moves")

	e.CheckOpenPRs("proj")
	got := strings.Join(prEventTypes(t, ps, "pr-sd-1"), "\n")
	if !strings.Contains(got, "precheck") {
		t.Fatalf("a PR whose base moved should carry a finding, got %q", got)
	}
	// Disjoint histories: it applies. The gate result is whatever this bare repo produces, so the
	// assertion is only that an applies-answer was recorded, not which gate verdict came with it.
	if strings.Contains(got, "precheck-conflict") {
		t.Errorf("nothing conflicts here, so no conflict should be reported: %q", got)
	}
}

// TestAConflictingPRIsReportedWithItsPaths: a finding the author cannot act on is not a finding, so
// the conflicting path has to be in it.
func TestAConflictingPRIsReportedWithItsPaths(t *testing.T) {
	e, ps, root := prCheckEngine(t)
	commitIn(t, filepath.Join(root, ".worktrees", "bombur"), "shared.txt", "the PR's line\n", "PR edits shared")
	commitIn(t, root, "shared.txt", "base's line\n", "base edits shared")

	e.CheckOpenPRs("proj")
	got := strings.Join(prEventTypes(t, ps, "pr-sd-1"), "\n")
	if !strings.Contains(got, "precheck-conflict") {
		t.Fatalf("competing edits should report a conflict, got %q", got)
	}
	if !strings.Contains(got, "shared.txt") {
		t.Errorf("the finding must name the conflicting path: %q", got)
	}
	if !strings.Contains(got, "main") {
		t.Errorf("the finding should say what it no longer applies onto: %q", got)
	}
}

// TestTheCheckIsAdvisory: no verdict, no status change, no interruption. Rejecting would route the
// PR back to its author mid-review, and on a busy base invite move → reject → rebase → resubmit.
func TestTheCheckIsAdvisory(t *testing.T) {
	e, ps, root := prCheckEngine(t)
	commitIn(t, filepath.Join(root, ".worktrees", "bombur"), "shared.txt", "the PR's line\n", "PR edits shared")
	commitIn(t, root, "shared.txt", "base's line\n", "base edits shared")
	deps := e.deps.(*stubDeps)

	e.CheckOpenPRs("proj")
	pr, _, err := ps.GetPR("pr-sd-1")
	if err != nil {
		t.Fatal(err)
	}
	if pr.Status != "submitted" {
		t.Errorf("the check must not change the PR's status, got %q", pr.Status)
	}
	if pr.Feedback != "" {
		t.Errorf("the check must not write a rejection, got %q", pr.Feedback)
	}
	if len(deps.injected) > 0 {
		t.Errorf("the author must not be interrupted mid-review, got %v", deps.injected)
	}
}

// TestTheSameTipsAreCheckedOnce is the debounce. A burst of merges leaves the base at one tip, and
// re-running a full gate per sweep for an answer already given is exactly the saturation this has to
// avoid. A further base move must of course be checked again.
func TestTheSameTipsAreCheckedOnce(t *testing.T) {
	e, ps, root := prCheckEngine(t)
	commitIn(t, root, "base-moved.txt", "later\n", "base moves")

	e.CheckOpenPRs("proj")
	first := len(prEventTypes(t, ps, "pr-sd-1"))
	if first == 0 {
		t.Fatal("precondition: the first pass should record a finding")
	}
	for i := 0; i < 3; i++ {
		e.CheckOpenPRs("proj") // further sweeps, nothing moved
	}
	if got := len(prEventTypes(t, ps, "pr-sd-1")); got != first {
		t.Errorf("the same tips were re-checked: %d findings, want %d", got, first)
	}
	// The base moves again: that is a new question, and it is asked.
	commitIn(t, root, "base-moved-again.txt", "later still\n", "base moves again")
	e.CheckOpenPRs("proj")
	if got := len(prEventTypes(t, ps, "pr-sd-1")); got <= first {
		t.Errorf("a further base move should be checked, still %d findings", got)
	}
}

// TestAMergedPRIsLeftAlone: the check is for work still in flight. A merged or scrapped PR has
// nothing to keep honest, and gating one would spend the run for no possible reader.
func TestAMergedPRIsLeftAlone(t *testing.T) {
	e, ps, root := prCheckEngine(t)
	if err := ps.PutPR(store.PR{ID: "pr-sd-1", Task: "sd-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "merged"}); err != nil {
		t.Fatal(err)
	}
	commitIn(t, root, "base-moved.txt", "later\n", "base moves")

	e.CheckOpenPRs("proj")
	if got := prEventTypes(t, ps, "pr-sd-1"); len(got) != 0 {
		t.Errorf("a merged PR should not be checked, got %v", got)
	}
}

// TestNoWorktreeIsLeftBehind: the throwaway is a reserved name, so a check that leaked it would
// break every check after it — and leave a stray tree in the user's repo.
func TestNoWorktreeIsLeftBehind(t *testing.T) {
	e, _, root := prCheckEngine(t)
	commitIn(t, root, "base-moved.txt", "later\n", "base moves")

	e.CheckOpenPRs("proj")
	if _, err := os.Stat(filepath.Join(root, ".worktrees", "precheck")); err == nil {
		t.Error("the throwaway worktree was left behind")
	}
}
