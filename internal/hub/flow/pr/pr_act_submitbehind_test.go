package pr

import (
	"bytes"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// submitEngine builds a worker mid-task on its own branch, ready to submit.
func submitEngine(t *testing.T) (*Act, *store.ProjectStore, string, registry.Caller) {
	t.Helper()
	root := t.TempDir()
	if err := exec.Command("git", "init", "-q", "-b", "main", root).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	flowtest.GitIn(t, root, "config", "user.email", "t@t")
	flowtest.GitIn(t, root, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	flowtest.GitIn(t, root, "add", "-A")
	flowtest.GitIn(t, root, "commit", "-q", "-m", "base")
	wt := filepath.Join(root, ".worktrees", "bombur")
	flowtest.GitIn(t, root, "worktree", "add", "-q", "-b", "sd-1", wt, "HEAD")
	flowtest.CommitIn(t, wt, "work.txt", "the worker's work\n", "did the work")

	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject(proj, root); err != nil {
		t.Fatal(err)
	}
	ps := st.For(proj)
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: ".worktrees/bombur"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "sd-1", Title: "the task", Status: "in_progress"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "sd-1", Branch: "sd-1", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	return newActOn(t, st, &flowtest.Hub{Root: root}), ps, root, registry.Caller{Project: proj, Agent: "bombur", Role: "worker"}
}

// TestSubmitRefusedWhenBehindTheBase is the defect. A PR recorded on a base the reference has moved
// past is stale the moment it exists: the human reviews and merges a diff against a state that is
// no longer there.
func TestSubmitRefusedWhenBehindTheBase(t *testing.T) {
	a, ps, root, c := submitEngine(t)
	flowtest.CommitIn(t, root, "base-moved.txt", "later\n", "base moves")

	var out bytes.Buffer
	code, err := a.CmdSubmit(c, []string{"my work"}, &out)
	if err != nil {
		t.Fatalf("a refusal is an answer, not an error: %v", err)
	}
	if code == 0 {
		t.Errorf("submitting behind the base should be refused, got exit 0:\n%s", out.String())
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); exists {
		t.Error("no PR may be recorded on a stale base — that is the whole point")
	}
	// The agent stays where it was: refusing must not park it in "submitted" awaiting a review of
	// something that does not exist.
	if got, _ := ps.GetState("bombur"); got.Phase != "working" {
		t.Errorf("a refused submit left the agent in %q", got.Phase)
	}
}

// TestTheRefusalSaysHowFarBehindAndWhatToRun: "rebase and try again" without the reason reads as a
// ritual. The count and the incoming commits are what let an agent judge whether its work still
// makes sense on top of what arrived.
func TestTheRefusalSaysHowFarBehindAndWhatToRun(t *testing.T) {
	a, _, root, c := submitEngine(t)
	flowtest.CommitIn(t, root, "first.txt", "a\n", "base moves once")
	flowtest.CommitIn(t, root, "second.txt", "b\n", "base moves twice")

	var out bytes.Buffer
	if _, err := a.CmdSubmit(c, nil, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"2 commit", "main", "sindri rebase", "sindri submit"} {
		if !strings.Contains(got, want) {
			t.Errorf("the refusal is missing %q:\n%s", want, got)
		}
	}
	// And what arrived, so the agent can see whether its work still holds.
	if !strings.Contains(got, "base moves twice") {
		t.Errorf("the refusal should name the commits that arrived:\n%s", got)
	}
}

// TestACurrentBranchStillSubmits is the other half: the guard must not stand between a worker and a
// PR it is entitled to create. A branch level with its base goes up exactly as before.
func TestACurrentBranchStillSubmits(t *testing.T) {
	a, ps, _, c := submitEngine(t)

	if code, out := submitAll(t, a, c, "my work"); code != 0 {
		t.Fatalf("a current branch should submit, got exit %d:\n%s", code, out)
	}
	runQueuedGate(t, a)
	pr, exists, _ := ps.GetPR("pr-sd-1")
	if !exists {
		t.Fatal("no PR was recorded for a current branch")
	}
	if pr.Base != "main" {
		t.Errorf("the PR records base %q, want main", pr.Base)
	}
}

// TestSubmittingAfterRebasingWorks is the path the refusal points at. If `rebase` then `submit` did
// not lead to a PR, the refusal would be a dead end rather than a redirection.
func TestSubmittingAfterRebasingWorks(t *testing.T) {
	a, ps, root, c := submitEngine(t)
	flowtest.CommitIn(t, root, "base-moved.txt", "later\n", "base moves")

	var refused bytes.Buffer
	if _, err := a.CmdSubmit(c, nil, &refused); err != nil {
		t.Fatal(err)
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); exists {
		t.Fatal("precondition: the first submit should have been refused")
	}
	// The worker does what it was told.
	var rb bytes.Buffer
	if _, err := a.CmdRebase(c, nil, &rb); err != nil {
		t.Fatalf("rebase: %v (%s)", err, rb.String())
	}
	if code, out := submitAll(t, a, c, "my work"); code != 0 {
		t.Fatalf("submit after rebase should succeed, got exit %d:\n%s", code, out)
	}
	runQueuedGate(t, a)
	if _, exists, _ := ps.GetPR("pr-sd-1"); !exists {
		t.Error("the path the refusal points at must end in a PR")
	}
}

// TestARefusedSubmitDoesNotCommit: the refusal comes before the worktree is committed, so a worker
// that rebases and submits again is committing the same work once, not layering a commit per
// refused attempt.
func TestARefusedSubmitDoesNotCommit(t *testing.T) {
	a, _, root, c := submitEngine(t)
	wt := filepath.Join(root, ".worktrees", "bombur")
	if err := os.WriteFile(filepath.Join(wt, "loose.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	flowtest.CommitIn(t, root, "base-moved.txt", "later\n", "base moves")
	before := revParseIn(t, wt, "HEAD")

	var out bytes.Buffer
	if _, err := a.CmdSubmit(c, nil, &out); err != nil {
		t.Fatal(err)
	}
	if after := revParseIn(t, wt, "HEAD"); after != before {
		t.Errorf("a refused submit committed the worktree: %s → %s", before, after)
	}
}

func revParseIn(t *testing.T, dir, ref string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", ref).Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

// TestAnEmptyBranchIsAskedToJustifyItself: a branch matching its base is ALLOWED — a container whose
// subtasks landed elsewhere is finished with nothing of its own to carry, which is dvalin's epic.
// Refusing it left the author no exit, since a live PR is the one thing that stops a task closing
// (-> prompts.ReplyPRStillToLand), and sudri resubmitted the same empty diff looking for one.
//
// So it is a question rather than a refusal, and the question is the one an empty diff actually
// raises: make the case that the work is done.
func TestAnEmptyBranchIsAskedToJustifyItself(t *testing.T) {
	a, _, root, caller := submitEngine(t)
	// Rewind the worker to the base: the shape a rebase leaves when the reference already carries
	// everything the branch was for.
	wt := filepath.Join(root, ".worktrees", "bombur")
	flowtest.GitIn(t, wt, "reset", "--hard", "main")

	var out bytes.Buffer
	if _, err := a.CmdSubmit(caller, []string{"nothing here"}, &out); err != nil {
		t.Fatalf("CmdSubmit: %v", err)
	}
	if !strings.Contains(out.String(), "changes nothing against its base") {
		t.Errorf("an empty branch should be asked to justify itself, got %q", out.String())
	}
	// Answered, it goes through: the author's case is on the record for the reviewer to weigh.
	if code, sout := submitAll(t, a, caller, "nothing here"); code != 0 {
		t.Fatalf("an answered empty submit should land, got %d: %s", code, sout)
	}
}

// submitAll drives CmdSubmit through its questions the way an agent does: the summary, then an
// answer per question, each arriving as its own `submit` call. Returns the final call's code and
// output — the one that either lands the submission or refuses it.
//
// A test that wants to see a QUESTION calls CmdSubmit directly; this is for the tests whose subject
// is what happens after the submit is taken.
func submitAll(t *testing.T, a *Act, c registry.Caller, summary string) (int, string) {
	t.Helper()
	const answer = "I swept the call sites this touches and each one is covered by a test that fails without it."
	text := summary
	for i := 0; i < 6; i++ { // a bound, so a flow that never settles fails loudly rather than hanging
		var out bytes.Buffer
		code, err := a.CmdSubmit(c, []string{text}, &out)
		if err != nil {
			t.Fatalf("CmdSubmit: %v", err)
		}
		// Both shapes mean the questionnaire is still running: a question put, or one put again
		// because the last answer was too short to be one.
		if !strings.Contains(out.String(), "Before this submit is taken") &&
			!strings.Contains(out.String(), "too short") {
			return code, out.String()
		}
		text = answer
	}
	t.Fatal("the submit questions never finished")
	return 0, ""
}

// TestAnEditedTaskCanStillBeFinished is sd-cc1aad's ONE THING TO CHECK, pinned rather than reasoned
// about: un-approving a task pauses its RELEASE, and the claim gate is about handing work out. A
// worker already mid-task must not be stranded because a planner corrected a line in its brief.
func TestAnEditedTaskCanStillBeFinished(t *testing.T) {
	a, ps, _, c := submitEngine(t)
	// The planner's edit, in the state it leaves behind: the held task is back awaiting a verdict.
	if err := ps.SetApproval("sd-1", "pending", ""); err != nil {
		t.Fatal(err)
	}

	if code, out := submitAll(t, a, c, "my work"); code != 0 {
		t.Fatalf("a held task must still submit while unapproved, got exit %d:\n%s", code, out)
	}
	runQueuedGate(t, a)
	if _, exists, _ := ps.GetPR("pr-sd-1"); !exists {
		t.Error("no PR was recorded — the worker was stranded by its own brief being corrected")
	}
}
