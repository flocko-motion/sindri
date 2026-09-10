// package: hub/flow/fleet / submit_fixture_test
// type:    logic (a worker with work to put up, on a real branch)
// job:     seed the shape a submit starts from — an agent holding a task, its worktree on that
// task's branch with an uncommitted change — so a test about what a submit LEAVES starts from a
// repo the gate can actually run in.
// limits:  seeding only.
package fleet

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// submitEngine builds a worker mid-task on its own branch, ready to submit.
func submitEngine(t *testing.T) (*Engine, *store.ProjectStore, string, registry.Caller) {
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
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "sd-1", Branch: "sd-1", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	return newEngine(t, st, &stubDeps{Root: root}), ps, root, registry.Caller{Project: "proj", Agent: "bombur", Role: "worker"}
}
