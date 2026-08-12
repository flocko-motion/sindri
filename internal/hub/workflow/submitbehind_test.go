package workflow

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// submitEngine builds a worker mid-task on its own branch, ready to submit.
func submitEngine(t *testing.T) (*Engine, *store.ProjectStore, string, registry.Caller) {
	t.Helper()
	root := t.TempDir()
	if err := exec.Command("git", "init", "-q", "-b", "main", root).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	run(t, root, "config", "user.email", "t@t")
	run(t, root, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(root, "shared.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, root, "add", "-A")
	run(t, root, "commit", "-q", "-m", "base")
	wt := filepath.Join(root, ".worktrees", "bombur")
	run(t, root, "worktree", "add", "-q", "-b", "sd-1", wt, "HEAD")
	commitIn(t, wt, "work.txt", "the worker's work\n", "did the work")

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
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "sd-1", Title: "the task", Status: "in_progress"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "sd-1", Branch: "sd-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	return New(st, &stubDeps{root: root}), ps, root, registry.Caller{Project: "proj", Agent: "bombur", Role: "worker"}
}

// TestSubmitRefusedWhenBehindTheBase is the defect. A PR recorded on a base the reference has moved
// past is stale the moment it exists: the human reviews and merges a diff against a state that is
// no longer there.
func TestSubmitRefusedWhenBehindTheBase(t *testing.T) {
	e, ps, root, c := submitEngine(t)
	commitIn(t, root, "base-moved.txt", "later\n", "base moves")

	var out bytes.Buffer
	code, err := e.CmdSubmit(c, []string{"my work"}, &out)
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
	e, _, root, c := submitEngine(t)
	commitIn(t, root, "first.txt", "a\n", "base moves once")
	commitIn(t, root, "second.txt", "b\n", "base moves twice")

	var out bytes.Buffer
	if _, err := e.CmdSubmit(c, nil, &out); err != nil {
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
	e, ps, _, c := submitEngine(t)

	var out bytes.Buffer
	code, err := e.CmdSubmit(c, []string{"my work"}, &out)
	if err != nil {
		t.Fatalf("CmdSubmit: %v", err)
	}
	if code != 0 {
		t.Fatalf("a current branch should submit, got exit %d:\n%s", code, out.String())
	}
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
	e, ps, root, c := submitEngine(t)
	commitIn(t, root, "base-moved.txt", "later\n", "base moves")

	var refused bytes.Buffer
	if _, err := e.CmdSubmit(c, nil, &refused); err != nil {
		t.Fatal(err)
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); exists {
		t.Fatal("precondition: the first submit should have been refused")
	}
	// The worker does what it was told.
	var rb bytes.Buffer
	if _, err := e.CmdRebase(c, nil, &rb); err != nil {
		t.Fatalf("rebase: %v (%s)", err, rb.String())
	}
	var out bytes.Buffer
	code, err := e.CmdSubmit(c, nil, &out)
	if err != nil {
		t.Fatalf("submit after rebase: %v", err)
	}
	if code != 0 {
		t.Fatalf("submit after rebase should succeed, got exit %d:\n%s", code, out.String())
	}
	if _, exists, _ := ps.GetPR("pr-sd-1"); !exists {
		t.Error("the path the refusal points at must end in a PR")
	}
}

// TestARefusedSubmitDoesNotCommit: the refusal comes before the worktree is committed, so a worker
// that rebases and submits again is committing the same work once, not layering a commit per
// refused attempt.
func TestARefusedSubmitDoesNotCommit(t *testing.T) {
	e, _, root, c := submitEngine(t)
	wt := filepath.Join(root, ".worktrees", "bombur")
	if err := os.WriteFile(filepath.Join(wt, "loose.txt"), []byte("uncommitted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitIn(t, root, "base-moved.txt", "later\n", "base moves")
	before := revParseIn(t, wt, "HEAD")

	var out bytes.Buffer
	if _, err := e.CmdSubmit(c, nil, &out); err != nil {
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
