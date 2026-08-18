package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// reviewerWithUnclaimedReview seeds a repo with one open, unclaimed review and a free reviewer —
// reviewDirective's own claiming path, before any prep gate.
func reviewerWithUnclaimedReview(t *testing.T, deps *stubDeps) (*Engine, *store.ProjectStore) {
	t.Helper()
	root := t.TempDir()
	deps.root = root
	st, err := store.Open(root + "/s.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "rune", Role: "reviewer"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	// UnclaimedReview requires the PR itself to read "open" — the status a review request leaves
	// it at, so anyone free can be handed it (-> RequestReview).
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "wrk", Branch: "td-1", Status: "open"}); err != nil {
		t.Fatalf("put pr: %v", err)
	}
	if _, err := ps.AddReview("pr-1", "check it"); err != nil {
		t.Fatalf("add review: %v", err)
	}
	return New(st, deps), ps
}

// TestFillPastTheCompactionThresholdCompactsThenHandsOverInOnePass: the assignment gate's whole
// point — fill past the (much lower than ContextFullFraction) compaction bar fires the compact
// inline, then hands the SAME assignment over in this same pass. No restart is involved, so there
// is nothing to wait on.
func TestFillPastTheCompactionThresholdCompactsThenHandsOverInOnePass(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-abc123") {
		t.Errorf("directive = %q, want the open task claimed in the same pass", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123", st.Task)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "dvalin" {
		t.Errorf("compacted = %v, want exactly one Compact(dvalin) fired ahead of the handover", deps.compacted)
	}
}

// TestFillUnderTheCompactionThresholdIsHandedWork is the control: an ordinary claim under threshold
// never fires a compact at all.
func TestFillUnderTheCompactionThresholdIsHandedWork(t *testing.T) {
	deps := &stubDeps{ctxTokens: 1_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
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
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — fill is under the threshold", deps.compacted)
	}
}

// TestClearWinsOverCompaction is rule 1 in the epic's order: an armed clear preempts compaction
// outright, even when fill is also past the compaction threshold — a summary made first would be
// discarded seconds later by the wipe the user asked for by hand. Unlike compaction, it fires
// whether or not an assignment exists, so it wins ahead of ever looking for one.
func TestClearWinsOverCompaction(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)
	a, _, err := ps.GetAgent("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	a.ClearArmed = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if strings.Contains(dir, "compact") || !strings.Contains(dir, "clear") {
		t.Errorf("directive = %q, want the clear-pending answer, not compaction", dir)
	}
	if len(deps.cleared) != 1 || deps.cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one FireClear(dvalin)", deps.cleared)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — the clear preempts it outright", deps.compacted)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("a clear-armed worker was handed %q anyway", st.Task)
	}
}

// TestRetiredIsExemptFromCompaction: a retired worker is exempt from every automatic behaviour, not
// just new task assignment — it must read "retired", not "compacting".
func TestRetiredIsExemptFromCompaction(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)
	a, _, err := ps.GetAgent("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "retired") {
		t.Errorf("directive = %q, want the retirement answer, not compaction", dir)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — retirement exempts it", deps.compacted)
	}
}

// TestCompactDueIgnoresAnUnknownWindow mirrors contextFull's own rule: a window nobody could
// resolve must never be treated as past a threshold computed from it.
func TestCompactDueIgnoresAnUnknownWindow(t *testing.T) {
	e := New(nil, &stubDeps{ctxTokens: 80_000, ctxWindow: 0, ctxOK: true, compactThreshold: 75_000})
	if _, due := e.compactDue("repo", "dvalin"); due {
		t.Error("a window of 0 must never read as past its own threshold")
	}
}

// TestBetweenSubtasksCompactsThenHandsOverInOnePass: the same one-pass rule claimNext follows, for
// a worker already holding a feature and about to be handed its next subtask (-> claimNextSubtask).
func TestBetweenSubtasksCompactsThenHandsOverInOnePass(t *testing.T) {
	const agent = "dain"
	root, _ := newWorkRepo(t, agent, "td-EPIC")
	st, err := store.Open(root + "/s.db")
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
	if err := ps.UpsertTask(store.Task{ID: "td-EPIC", Title: "a feature", Status: "open", Priority: "P1", Type: "epic"}); err != nil {
		t.Fatalf("seed feature: %v", err)
	}
	// OpenSubtasks reads the synced cache table, not owned_tasks directly — both are seeded by hand,
	// mirroring what a real sync keeps in step.
	if err := ps.UpsertTask(store.Task{ID: "td-next", Title: "the next subtask", Status: "open", Priority: "P1", ParentID: "td-EPIC"}); err != nil {
		t.Fatalf("seed subtask cache: %v", err)
	}
	if err := ps.PutOwnedTask(store.OwnedTask{ID: "td-next", Title: "the next subtask", Status: "open"}); err != nil {
		t.Fatalf("seed subtask: %v", err)
	}
	if err := ps.SetParent("td-next", "td-EPIC"); err != nil {
		t.Fatalf("set parent: %v", err)
	}
	// Between subtasks: the feature is held, but nothing is currently assigned within it.
	if err := ps.SetState(store.AgentState{Agent: agent, Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatalf("set state: %v", err)
	}

	deps := &stubDeps{root: root, ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := New(st, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-next") {
		t.Errorf("directive = %q, want the next subtask claimed in the same pass", dir)
	}
	if held, _ := ps.GetState(agent); held.Task != "td-next" {
		t.Errorf("state.Task = %q, want td-next", held.Task)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != agent {
		t.Errorf("compacted = %v, want exactly one Compact(%s) fired ahead of the handover", deps.compacted, agent)
	}
}

// TestReviewerFillPastTheCompactionThresholdCompactsThenHandsOverInOnePass: the same rule, for a
// reviewer about to be handed a new PR rather than a worker about to be handed a new task.
func TestReviewerFillPastTheCompactionThresholdCompactsThenHandsOverInOnePass(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := reviewerWithUnclaimedReview(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "pr-1") {
		t.Errorf("directive = %q, want the review claimed in the same pass", dir)
	}
	if held, _ := ps.ReviewingPR("rune"); held != "pr-1" {
		t.Errorf("ReviewingPR = %q, want pr-1", held)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "rune" {
		t.Errorf("compacted = %v, want exactly one Compact(rune) fired ahead of the handover", deps.compacted)
	}
}

// TestReviewerFillUnderTheCompactionThresholdIsHandedAReview is the control: an ordinary claim under
// threshold never fires a compact at all.
func TestReviewerFillUnderTheCompactionThresholdIsHandedAReview(t *testing.T) {
	deps := &stubDeps{ctxTokens: 1_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := reviewerWithUnclaimedReview(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "pr-1") {
		t.Errorf("directive = %q, want the open review claimed", dir)
	}
	if held, _ := ps.ReviewingPR("rune"); held != "pr-1" {
		t.Errorf("ReviewingPR = %q, want pr-1", held)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — fill is under the threshold", deps.compacted)
	}
}
