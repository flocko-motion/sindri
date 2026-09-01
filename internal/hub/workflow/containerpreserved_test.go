package workflow

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// Regression tests for sd-5ef393, folded into sd-a72056's reason-carrying mutator: SetState writes
// the whole row, so a caller that reset an agent without carrying its Container forward silently
// unhooked it from the feature it still held. Three sites did — finishTask (via CloseTask),
// UnassignTask, and DiscardPR — each fixed to rest a container holder back onto its feature
// (Container/Branch preserved) rather than fully idle.

// TestCloseTaskPreservesAHeldContainer: closing a SUBTASK under a held feature must rest the worker
// back onto the feature, not wipe it to bare idle — the feature itself is not done.
func TestCloseTaskPreservesAHeldContainer(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-sub", Title: "a subtask", Status: "open", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutAgent(store.Agent{Name: "brokkr", Role: "worker", Workspace: ".worktrees/brokkr"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "brokkr", Task: "td-sub", Container: "td-feature", Branch: "td-feature", Phase: "working"},
		store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	e := newEngine(st, &stubDeps{root: root})

	if err := e.CloseTask("proj", "td-sub"); err != nil {
		t.Fatalf("CloseTask: %v", err)
	}
	got, err := ps.GetState("brokkr")
	if err != nil {
		t.Fatal(err)
	}
	if got.Container != "td-feature" || got.Branch != "td-feature" {
		t.Errorf("closing a subtask must preserve the held feature, got Container=%q Branch=%q", got.Container, got.Branch)
	}
	if got.Task != "" {
		t.Errorf("the closed subtask itself must be cleared, got Task=%q", got.Task)
	}
	if got.Phase != "idle" {
		t.Errorf("phase should rest at idle within the feature, got %q", got.Phase)
	}
}

// TestUnassignTaskPreservesAHeldContainer: unassigning one subtask must not strand the worker out of
// the feature it still holds — the same shape as CloseTask's own fix.
func TestUnassignTaskPreservesAHeldContainer(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-sub", Title: "a subtask", Status: "open", Priority: "P2"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutAgent(store.Agent{Name: "brokkr", Role: "worker", Workspace: ".worktrees/brokkr"}); err != nil {
		t.Fatal(err)
	}
	// Down (not alive): UnassignTask refuses a live holder, so the crashed-mid-feature case is the
	// one worth pinning — a stale claim under a held container must still keep the container.
	if err := ps.SetState(store.AgentState{Agent: "brokkr", Task: "td-sub", Container: "td-feature", Branch: "td-feature", Phase: "working"},
		store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	e := newEngine(st, &stubDeps{root: root, alive: false})

	if err := e.UnassignTask("proj", "td-sub"); err != nil {
		t.Fatalf("UnassignTask: %v", err)
	}
	got, err := ps.GetState("brokkr")
	if err != nil {
		t.Fatal(err)
	}
	if got.Container != "td-feature" || got.Branch != "td-feature" {
		t.Errorf("unassigning a subtask must preserve the held feature, got Container=%q Branch=%q", got.Container, got.Branch)
	}
	if got.Task != "" {
		t.Errorf("the unassigned subtask itself must be cleared, got Task=%q", got.Task)
	}
}

// TestDiscardPRPreservesAHeldContainer: discarding a milestone/interim PR from a container worker
// must not drop it out of the feature — only that PR is going away, not the feature it belongs to.
func TestDiscardPRPreservesAHeldContainer(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "brokkr", Role: "worker", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-td-feature", Task: "td-feature", Agent: "brokkr", Branch: "td-feature", Status: "open", Kind: "interim"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "brokkr", Task: "td-sub", Container: "td-feature", Branch: "td-feature", Phase: "submitted"},
		store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: root, alive: true}
	e := newEngine(st, deps)

	if err := e.DiscardPR("proj", "pr-td-feature"); err != nil {
		t.Fatalf("DiscardPR: %v", err)
	}
	got, err := ps.GetState("brokkr")
	if err != nil {
		t.Fatal(err)
	}
	if got.Container != "td-feature" || got.Branch != "td-feature" {
		t.Errorf("discarding a milestone PR must preserve the held feature, got Container=%q Branch=%q", got.Container, got.Branch)
	}
	if got.Phase != "idle" {
		t.Errorf("phase should rest at idle within the feature, got %q", got.Phase)
	}
}
