package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAStrandedContainerIsClaimedToFinishIt is the fix for the hole a task with no live claim
// path leaves behind: a package whose only child has already closed, and whose own branch never
// went up as a PR, has nothing OpenLeaves or OpenContainers would ever have offered before this —
// permanently. claimNext must offer it, and claiming it must hold the EXISTING branch (reusing
// whatever the closed subtask left there) rather than resetting onto a fresh one.
func TestAStrandedContainerIsClaimedToFinishIt(t *testing.T) {
	const agent = "dvalin"
	root, _ := newWorkRepo(t, agent, "td-pkg") // lays the branch the closed subtask worked on
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "worker", Workspace: ".worktrees/" + agent}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-pkg", Title: "a package", Status: "open", Priority: "P1", Type: "epic"}); err != nil {
		t.Fatalf("seed package: %v", err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-pkg-c1", Title: "its only child", Status: "closed", Priority: "P1"}); err != nil {
		t.Fatalf("seed closed child: %v", err)
	}
	if err := ps.SetParent("td-pkg-c1", "td-pkg"); err != nil {
		t.Fatalf("set parent: %v", err)
	}
	// Idle, holding nothing — an agent free to claim, the state after a prior holder finished the
	// last checkpoint and moved on without ever submitting the branch.
	if err := ps.SetState(store.AgentState{Agent: agent, Phase: "idle"}); err != nil {
		t.Fatalf("set state: %v", err)
	}

	e := New(st, &stubDeps{root: root}) // claimNext's own SyncTasks lays the owned rows into the cache
	dir, err := e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-pkg") || !strings.Contains(dir, "sindri submit") {
		t.Errorf("directive = %q, want it to name td-pkg and tell the agent to submit", dir)
	}

	held, _ := ps.GetState(agent)
	if held.Container != "td-pkg" {
		t.Fatalf("state.Container = %q, want td-pkg — the stranded package must be held, not skipped", held.Container)
	}
	if held.Task != "" {
		t.Errorf("state.Task = %q, want empty — nothing was assigned to work, only the package to finish", held.Task)
	}
}
