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
	if err := ps.SetState(store.AgentState{Agent: agent, Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	return newEngine(st, deps), ps
}

// TestAFullWorkerIsClearedAndPreparedForTheNextTask is why the retirement gate went: asking for
// work IS a leaf boundary, which is exactly where a clear is safe, so a full worker is claimed for
// like any other and prepared with a clear instead of being refused and left waiting on a human.
func TestAFullWorkerIsClearedAndPreparedForTheNextTask(t *testing.T) {
	deps := &stubDeps{ctxTokens: 900_000, ctxWindow: 1_000_000, ctxOK: true}
	e, ps := idleWorkerWithOpenTask(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — a full worker is prepared, not refused", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123 — the claim holds regardless of what this ask answers", st.Task)
	}
	if len(deps.cleared) != 1 || deps.cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one Clear(dvalin) fired in place of a compaction", deps.cleared)
	}
	if len(deps.injectedText) != 1 || !strings.Contains(deps.injectedText[0], "td-abc123") {
		t.Errorf("injectedText = %v, want the claimed directive delivered once the clear answered", deps.injectedText)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — past ContextFullFraction clears rather than compacts", deps.compacted)
	}
}

// TestAWorkerUnderTheThresholdIsHandedWork is the control: nothing about the fullness gate should
// stop an ordinary claim from working exactly as it always has.
func TestAWorkerUnderTheThresholdIsHandedWork(t *testing.T) {
	deps := &stubDeps{ctxTokens: 1000, ctxWindow: 200_000, ctxOK: true}
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
// ContextUsage, which must read as "not full" rather than as full-by-default.
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

// TestFullnessIsRelativeToTheWindow is the bug this replaced: 480k fills a 200k window and is half
// a 1M one, so the same count must answer differently. Against a flat 170k the whole fleet read full.
func TestFullnessIsRelativeToTheWindow(t *testing.T) {
	for _, c := range []struct {
		what           string
		tokens, window int
		wantFull       bool
	}{
		{"half of a 1M window", 480_000, 1_000_000, false},
		{"the same count against 200k", 480_000, 200_000, true},
		{"just under the fraction", 840_000, 1_000_000, false},
		{"just over it", 860_000, 1_000_000, true},
		// A window nobody could resolve must not retire anyone: guessing one is what this replaced.
		{"measured, but no window known", 480_000, 0, false},
	} {
		e := newEngine(nil, &stubDeps{ctxTokens: c.tokens, ctxWindow: c.window, ctxOK: true})
		if _, full := e.contextFull("repo", "dvalin"); full != c.wantFull {
			t.Errorf("%s: %d of %d full=%v, want %v", c.what, c.tokens, c.window, full, c.wantFull)
		}
	}
}

// TestFullnessGatesNewWorkOnly is the boundary the gate must respect: it withholds the NEXT task,
// never the one already held. Retiring a worker whose PR bounced would strand finished work behind
// a review nobody could answer.
func TestFullnessGatesNewWorkOnly(t *testing.T) {
	full := &stubDeps{ctxTokens: 990_000, ctxWindow: 1_000_000, ctxOK: true}
	e, ps := idleWorkerWithOpenTask(t, full)

	// Holding a task: the directive is the task's, not a retirement notice.
	if err := ps.SetState(store.AgentState{
		Agent: "dvalin", Task: "td-abc123", Branch: "td-abc123", Phase: "working",
	}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if strings.Contains(dir, "retired") || !strings.Contains(dir, "td-abc123") {
		t.Errorf("a full worker mid-task must keep working on it, got: %q", dir)
	}

	// Its PR then bounces. The feedback must reach it, full or not.
	if err := ps.PutPR(store.PR{
		ID: "pr-td-abc123", Task: "td-abc123", Agent: "dvalin", Branch: "td-abc123",
		Status: "rejected", Feedback: "needs a test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{
		Agent: "dvalin", Task: "td-abc123", Branch: "td-abc123", Phase: "submitted",
	}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	dir, err = e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if strings.Contains(dir, "retired") {
		t.Errorf("a rejection must reach a full worker — it is the work it already holds: %q", dir)
	}
	if !strings.Contains(dir, "needs a test") {
		t.Errorf("the directive should carry the reviewer's feedback: %q", dir)
	}
}
