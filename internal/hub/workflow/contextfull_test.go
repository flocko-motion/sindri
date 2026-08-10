package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// idleWorkerWithOpenTask seeds a repo with one open, approved, prioritized leaf task and an idle
// worker with a worktree ready to claim it — claimNext's happy path, before any fullness gate.
func idleWorkerWithOpenTask(t *testing.T, deps *stubDeps) (*Engine, *store.ProjectStore) {
	t.Helper()
	const agent = "dvalin"
	root, _ := newWorkRepo(t, agent, "seed")
	deps.root = root
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
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-abc123", Title: "a task", Status: "open", Priority: "P2"}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Phase: "idle"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	return New(st, deps), ps
}

// TestAFullWorkerIsNotHandedTheNextTask is RETIRE's whole point: past the threshold, an open leaf
// sits there unclaimed rather than landing on a worker who has no room left for it.
func TestAFullWorkerIsNotHandedTheNextTask(t *testing.T) {
	deps := &stubDeps{ctxTokens: ContextFullThreshold, ctxOK: true}
	e, ps := idleWorkerWithOpenTask(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "retired") {
		t.Errorf("directive = %q, want it to say the worker is retired for a full context", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("a full worker was handed %q — it must stay unclaimed", st.Task)
	}
}

// TestAWorkerUnderTheThresholdIsHandedWork is the control: nothing about the fullness gate should
// stop an ordinary claim from working exactly as it always has.
func TestAWorkerUnderTheThresholdIsHandedWork(t *testing.T) {
	deps := &stubDeps{ctxTokens: 1000, ctxOK: true}
	e, ps := idleWorkerWithOpenTask(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-abc123") {
		t.Errorf("directive = %q, want the open task claimed", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123", st.Task)
	}
}

// TestNoRecordedUsageIsNeverFull: an agent that has never replied has ok=false from
// ContextTokens, which must read as "not full" rather than as full-by-default.
func TestNoRecordedUsageIsNeverFull(t *testing.T) {
	deps := &stubDeps{ctxOK: false}
	e, ps := idleWorkerWithOpenTask(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-abc123") {
		t.Errorf("directive = %q, want the open task claimed (no usage recorded yet != full)", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123", st.Task)
	}
}
