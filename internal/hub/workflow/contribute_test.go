package workflow

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// newWorkRepo builds a throwaway git repo with one commit on the default branch and a
// feature-branch worktree at .worktrees/<agent> checked out from it — the shape the
// hub lays down for a worker. Returns the repo root and the base branch name.
func newWorkRepo(t *testing.T, agent, branch string) (root, base string) {
	t.Helper()
	root = t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run(root, "init", "-q")
	run(root, "config", "user.email", "t@t")
	run(root, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(root, "seed"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(root, "add", ".")
	run(root, "commit", "-qm", "init")
	base, err := git.CurrentBranch(root)
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, ".worktrees", agent)
	if err := git.WorktreeAdd(root, wt, "HEAD"); err != nil {
		t.Fatalf("worktree add: %v", err)
	}
	if err := git.CreateBranch(wt, branch, base); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	return root, base
}

// TestContributeThenMergeKeepsTaskOpen is the interim-contribution happy path:
// `contribute` records a GATED interim PR (open, no reviewer requested), and merging
// it keeps the task open and puts the worker straight back to "working" on the SAME
// task — the point of a mid-task contribution.
func TestContributeThenMergeKeepsTaskOpen(t *testing.T) {
	const agent, task, branch = "wrk", "td-1", "td-1"
	root, base := newWorkRepo(t, agent, branch)

	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: filepath.Join(".worktrees", agent)}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Task: task, Branch: branch, Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	// The worker made progress it wants to land mid-task.
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
	if pr.Kind != "interim" {
		t.Fatalf("PR kind = %q, want interim", pr.Kind)
	}
	if pr.Status != "open" {
		t.Fatalf("PR status = %q, want open (gated, awaiting the user)", pr.Status)
	}
	if revs, _ := ps.Reviews(pr.ID); len(revs) != 0 {
		t.Fatalf("interim PR must not request a reviewer, got %d review(s)", len(revs))
	}
	// The worker waits after contributing.
	if s, _ := ps.GetState(agent); s.Phase != "submitted" {
		t.Fatalf("worker phase after contribute = %q, want submitted", s.Phase)
	}

	// The user approves, then merges.
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Merge("repo", pr.ID); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	merged, _, _ := ps.GetPR(pr.ID)
	if merged.Status != "merged" {
		t.Fatalf("PR status after merge = %q, want merged", merged.Status)
	}
	// The task stays with the worker and it's back to working — NOT idled/closed.
	s, _ := ps.GetState(agent)
	if s.Phase != "working" || s.Task != task {
		t.Fatalf("worker after interim merge = {phase:%q task:%q}, want {working %s}", s.Phase, s.Task, task)
	}
	_ = base
}
