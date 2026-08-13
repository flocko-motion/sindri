package workflow

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

func TestCCType(t *testing.T) {
	for _, tc := range []struct{ taskType, want string }{
		{"bug", "fix"},
		{"issue", "fix"},
		{"feature", "feat"},
		{"epic", "feat"},
		{"chore", "chore"},
		{"task", "chore"},
		{"", "chore"},
		{"something-unknown", "chore"},
	} {
		if got := ccType(tc.taskType); got != tc.want {
			t.Errorf("ccType(%q) = %q, want %q", tc.taskType, got, tc.want)
		}
	}
}

func TestConventionalCommit(t *testing.T) {
	if got, want := conventionalCommit("bug", "sd-1", "fix the thing"), "fix(sd-1): fix the thing"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := conventionalCommit("", "", "openspec update"), "chore: openspec update"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// lastCommitMsg reads the subject of the last commit in dir, so a test can check the shape sindri
// actually wrote to git rather than trusting the string it composed.
func lastCommitMsg(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "log", "-1", "--format=%s").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// TestSubmitCommitsConventionally is the regression for the reported bug: every commit sindri
// creates must pass a Conventional Commits check, deriving its type from the task's own declared
// type rather than a fixed prefix that would flatten every landing into "chore".
func TestSubmitCommitsConventionally(t *testing.T) {
	const agent, task, branch = "wrk", "sd-1", "sd-1"
	root, _ := newWorkRepo(t, agent, branch)

	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: task, Title: "fix the flaky retry", Type: "bug", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Task: task, Branch: branch, Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, ".worktrees", agent)
	if err := os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("patched\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := New(st, &stubDeps{root: root})
	c := registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: "working"}

	var out strings.Builder
	if code, err := e.CmdSubmit(c, []string{"retry", "with", "backoff"}, &out); err != nil || code != 0 {
		t.Fatalf("CmdSubmit: code=%d err=%v out=%s", code, err, out.String())
	}
	if got, want := lastCommitMsg(t, wt), "fix(sd-1): retry with backoff"; got != want {
		t.Errorf("commit message = %q, want %q", got, want)
	}
}

// TestCheckpointFallsBackToTaskTitle covers the no-summary case: sindri must still produce a
// conforming commit, describing it with the task's own title rather than an unreadable id.
func TestCheckpointFallsBackToTaskTitle(t *testing.T) {
	const agent = "dain"
	root, _ := newWorkRepo(t, agent, "td-EPIC")
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatal(err)
	}
	for _, task := range []store.Task{
		{ID: "td-EPIC", Title: "a feature", Status: "open", Type: "epic"},
		{ID: "td-1", Title: "wire up the retry path", Status: "open", Type: "feature", ParentID: "td-EPIC"},
	} {
		if err := ps.UpsertTask(task); err != nil {
			t.Fatal(err)
		}
	}
	// A checkpoint closes its subtask through the source (finishAtSource), so td-1 needs an owned
	// row and a recorded parent — the same seeding nestedfeature_test.go uses.
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-1", Title: "wire up the retry path", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetParent("td-1", "td-EPIC"); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{
		Agent: agent, Container: "td-EPIC", Branch: "td-EPIC", Task: "td-1", Phase: "working",
	}); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, ".worktrees", agent)
	if err := os.WriteFile(filepath.Join(wt, "retry.txt"), []byte("wired\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := New(st, &stubDeps{root: root})
	c := registry.Caller{Project: "repo", Agent: agent, Role: "worker", Phase: "working"}
	var out strings.Builder
	if code, err := e.CmdCheckpoint(c, nil, &out); err != nil || code != 0 {
		t.Fatalf("CmdCheckpoint: code=%d err=%v out=%s", code, err, out.String())
	}
	if got, want := lastCommitMsg(t, wt), "feat(td-1): wire up the retry path"; got != want {
		t.Errorf("commit message = %q, want %q", got, want)
	}
}

// TestMergeCommitIsConventional is the exact regression from the report ("Commit 3a406f95: merge
// sd-817b3a"): the merge commit is the one landing representing the whole PR, and unlike a
// checkpoint or submit it has no agent-supplied text to wrap — it must name the task's own title.
func TestMergeCommitIsConventional(t *testing.T) {
	const agent, task, branch = "wrk", "td-1", "td-1"
	root, _ := newWorkRepo(t, agent, branch)

	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: task, Title: "stop the retry storm", Type: "bug", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Task: task, Branch: branch, Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".worktrees", agent, "work.txt"), []byte("progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := New(st, &stubDeps{root: root})
	caller := registry.Caller{Project: "repo", Agent: agent, Role: "worker", HasTask: true, Phase: "working"}
	if code, err := e.CmdContribute(caller, []string{"checkpoint"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdContribute: code=%d err=%v", code, err)
	}
	pr, ok, _ := ps.GetPR("pr-" + task)
	if !ok {
		t.Fatal("contribute should have created pr-td-1")
	}
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Merge("repo", pr.ID); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got, want := lastCommitMsg(t, root), "fix(td-1): stop the retry storm"; got != want {
		t.Errorf("merge commit message = %q, want %q", got, want)
	}
}
