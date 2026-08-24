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

// checkOpenPRs sweeps and then drains what it queued. The sweep only DECIDES that a check should
// happen — the materialise-and-gate is a queued run now, sharing the fleet's one slot with every
// other gate, so a test that stopped at the sweep would assert against a check that never ran.
func checkOpenPRs(t *testing.T, e *Engine) {
	t.Helper()
	e.CheckOpenPRs("proj")
	for {
		project, id, ok := e.NextQueuedRun()
		if !ok {
			return
		}
		if err := e.ExecuteRun(t.Context(), project, id); err != nil {
			t.Fatalf("ExecuteRun(%s): %v", id, err)
		}
	}
}

// run is a git command that must succeed.
func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
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
	checkOpenPRs(t, e)
	if got := prEventTypes(t, ps, "pr-sd-1"); len(got) != 0 {
		t.Errorf("a PR level with its base needs no check, got %v", got)
	}
}

// TestABehindPRIsCheckedAndTheFindingLandsOnThePR: the whole point. After the base moves, the PR
// carries the answer where the human reading it will see it.
func TestABehindPRIsCheckedAndTheFindingLandsOnThePR(t *testing.T) {
	e, ps, root := prCheckEngine(t)
	commitIn(t, root, "base-moved.txt", "later\n", "base moves")

	checkOpenPRs(t, e)
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

	checkOpenPRs(t, e)
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

	checkOpenPRs(t, e)
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

	checkOpenPRs(t, e)
	first := len(prEventTypes(t, ps, "pr-sd-1"))
	if first == 0 {
		t.Fatal("precondition: the first pass should record a finding")
	}
	for i := 0; i < 3; i++ {
		checkOpenPRs(t, e) // further sweeps, nothing moved
	}
	if got := len(prEventTypes(t, ps, "pr-sd-1")); got != first {
		t.Errorf("the same tips were re-checked: %d findings, want %d", got, first)
	}
	// The base moves again: that is a new question, and it is asked.
	commitIn(t, root, "base-moved-again.txt", "later still\n", "base moves again")
	checkOpenPRs(t, e)
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

	checkOpenPRs(t, e)
	if got := prEventTypes(t, ps, "pr-sd-1"); len(got) != 0 {
		t.Errorf("a merged PR should not be checked, got %v", got)
	}
}

// TestNoWorktreeIsLeftBehind: the throwaway is a reserved name, so a check that leaked it would
// break every check after it — and leave a stray tree in the user's repo.
func TestNoWorktreeIsLeftBehind(t *testing.T) {
	e, _, root := prCheckEngine(t)
	commitIn(t, root, "base-moved.txt", "later\n", "base moves")

	checkOpenPRs(t, e)
	if _, err := os.Stat(filepath.Join(root, ".worktrees", "precheck")); err == nil {
		t.Error("the throwaway worktree was left behind")
	}
}

// TestTheCheckUsesThePRsOwnBase is the base the MERGE will use. repo.MergeBranch replays onto
// pr.Base and names it in every conflict; a check that used the project's current reference instead
// would answer about an operation nobody performs — and this check is triggered by the very event
// that makes the two diverge, a reference being moved or re-pointed. Worse than a wrong label: a PR
// that conflicts with its real base can come back clean.
func TestTheCheckUsesThePRsOwnBase(t *testing.T) {
	e, ps, root := prCheckEngine(t)
	// A second base, which is what the PR is recorded against — while the project reference (the
	// repo's current branch, main) is something else entirely.
	run(t, root, "branch", "release", "HEAD")
	if err := ps.PutPR(store.PR{ID: "pr-sd-1", Task: "sd-1", Agent: "bombur", Branch: "sd-1", Base: "release", Status: "submitted"}); err != nil {
		t.Fatal(err)
	}
	// Only the PR's real base moves, and it conflicts. main stays where it was.
	run(t, root, "checkout", "-q", "release")
	commitIn(t, root, "shared.txt", "release's line\n", "release edits shared")
	run(t, root, "checkout", "-q", "main")
	commitIn(t, filepath.Join(root, ".worktrees", "bombur"), "shared.txt", "the PR's line\n", "PR edits shared")

	checkOpenPRs(t, e)
	got := strings.Join(prEventTypes(t, ps, "pr-sd-1"), "\n")
	if !strings.Contains(got, "precheck-conflict") {
		t.Fatalf("the conflict is with the PR's own base, and must be found: %q", got)
	}
	if !strings.Contains(got, "release") {
		t.Errorf("the finding must name the PR's base, got %q", got)
	}
	if strings.Contains(got, "main") {
		t.Errorf("the finding names a base this PR will not be merged onto: %q", got)
	}
}

// TestAPRIsRecheckedWhenItsBaseChanges: the memo carries the base NAME, not just tips. Re-pointing a
// PR at another branch is a new question even when nothing has moved, and a memo keyed on tips alone
// would answer it with the verdict from the old base.
func TestAPRIsRecheckedWhenItsBaseChanges(t *testing.T) {
	e, ps, root := prCheckEngine(t)
	// Both bases move past the PR, so it is behind either one — otherwise the second check would be
	// skipped for being level, and the test would pass without exercising the memo at all.
	run(t, root, "branch", "release", "HEAD")
	run(t, root, "checkout", "-q", "release")
	commitIn(t, root, "release-moved.txt", "later\n", "release moves")
	run(t, root, "checkout", "-q", "main")
	commitIn(t, root, "base-moved.txt", "later\n", "main moves")

	checkOpenPRs(t, e)
	first := len(prEventTypes(t, ps, "pr-sd-1"))
	if first == 0 {
		t.Fatal("precondition: the first check should record a finding")
	}
	// Re-point the PR at a different base. Nothing moved; the question is new.
	pr, _, err := ps.GetPR("pr-sd-1")
	if err != nil {
		t.Fatal(err)
	}
	pr.Base = "release"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	checkOpenPRs(t, e)
	if got := len(prEventTypes(t, ps, "pr-sd-1")); got <= first {
		t.Errorf("a PR re-pointed at another base should be re-checked, still %d findings", got)
	}
}

// TestARepointedPRIsRecheckedEvenAtTheSameTip is the narrow case the base NAME in the memo key
// exists for. Two branches can sit on the same commit, so re-pointing a PR between them changes no
// tip at all — but the finding names the base, so a memo keyed on tips alone would suppress the
// re-check and leave the PR carrying a verdict about a branch it is no longer aimed at.
func TestARepointedPRIsRecheckedEvenAtTheSameTip(t *testing.T) {
	e, ps, root := prCheckEngine(t)
	commitIn(t, root, "base-moved.txt", "later\n", "main moves")
	run(t, root, "branch", "release", "main") // a second name for the same commit

	checkOpenPRs(t, e)
	got := strings.Join(prEventTypes(t, ps, "pr-sd-1"), "\n")
	if !strings.Contains(got, "main") {
		t.Fatalf("precondition: the first finding should name main, got %q", got)
	}
	first := len(prEventTypes(t, ps, "pr-sd-1"))

	pr, _, err := ps.GetPR("pr-sd-1")
	if err != nil {
		t.Fatal(err)
	}
	pr.Base = "release" // same commit, different branch
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	checkOpenPRs(t, e)
	if len(prEventTypes(t, ps, "pr-sd-1")) <= first {
		t.Fatal("a PR aimed at a different base must be re-checked, even at an identical tip")
	}
	if latest := prEventTypes(t, ps, "pr-sd-1"); !strings.Contains(latest[len(latest)-1], "release") {
		t.Errorf("the new finding should name the new base, got %q", latest[len(latest)-1])
	}
}
