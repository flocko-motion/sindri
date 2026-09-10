package fleet

import (
	"context"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// idleWorkerWithOpenTask seeds a repo with one open, approved, prioritized leaf task and an idle
// worker with a worktree ready to claim it — ClaimNext's happy path, before any fullness gate.
func idleWorkerWithOpenTask(t *testing.T, deps *stubDeps) (*Engine, *store.ProjectStore) {
	t.Helper()
	const agent = "dvalin"
	root, _ := flowtest.WorkRepo(t, agent, "seed")
	deps.Root = root
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
	e := newEngine(t, st, deps)
	// The owned row is the source; the cached read model is what every claim and every pass reads,
	// and only a sync carries one to the other.
	if err := e.taskAct().SyncTasks("repo"); err != nil {
		t.Fatalf("sync tasks: %v", err)
	}
	return e, ps
}

// TestAFullWorkerIsClearedAndPreparedForTheNextTask is why the retirement gate went: asking for
// work IS a leaf boundary, which is exactly where a clear is safe, so a full worker is claimed for
// like any other and prepared with a clear instead of being refused and left waiting on a human.
func TestAFullWorkerIsClearedAndPreparedForTheNextTask(t *testing.T) {
	deps := &stubDeps{CtxTokens: 900_000, CtxWindow: 1_000_000, CtxOK: true}
	e, ps := idleWorkerWithOpenTask(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if strings.Contains(dir, "One moment") || prompts.Deferring(dir) {
		t.Errorf("directive = %q, want the work itself — the ask settles before it reports, so a "+
			"placeholder is no longer an answer anybody has to be given", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123 — the claim holds regardless of what this ask answers", st.Task)
	}
	if len(deps.Cleared) != 1 || deps.Cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one Clear(dvalin) before the work is handed over", deps.Cleared)
	}
	if len(deps.InjectedText) != 1 || !strings.Contains(deps.InjectedText[0], "td-abc123") {
		t.Errorf("injectedText = %v, want the claimed directive delivered once the clear answered", deps.InjectedText)
	}
}

// TestAnEmptyWorkerSessionIsHandedWork is the control: an ordinary claim onto a session with nothing
// in it works exactly as it always has, with no preparation in the way.
func TestAnEmptyWorkerSessionIsHandedWork(t *testing.T) {
	deps := &stubDeps{CtxTokens: 0, CtxWindow: 200_000, CtxOK: true}
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
	deps := &stubDeps{CtxOK: false}
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
		e := storelessEngine(t, &stubDeps{CtxTokens: c.tokens, CtxWindow: c.window, CtxOK: true})
		if _, full := e.roleAct().ContextFull("repo", "dvalin"); full != c.wantFull {
			t.Errorf("%s: %d of %d full=%v, want %v", c.what, c.tokens, c.window, full, c.wantFull)
		}
	}
}

// TestFullnessGatesNewWorkOnly is the boundary the gate must respect: it withholds the NEXT task,
// never the one already held. Retiring a worker whose PR bounced would strand finished work behind
// a review nobody could answer.
func TestFullnessGatesNewWorkOnly(t *testing.T) {
	full := &stubDeps{CtxTokens: 990_000, CtxWindow: 1_000_000, CtxOK: true}
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
