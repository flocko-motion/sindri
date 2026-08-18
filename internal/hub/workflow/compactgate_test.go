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

// TestFillPastTheCompactionThresholdEndsThePassThenHandsOverOnTheNextAsk: fill past the (much lower
// than ContextFullFraction) compaction bar fires the compact and ends the pass right there — never
// handing the SAME assignment over into the un-compacted context it exists to avoid. The task is
// claimed only once a later ask finds the fresh reading (the queued /compact having landed) below
// the threshold — the boundary re-ask, not this call, is what claims it.
func TestFillPastTheCompactionThresholdEndsThePassThenHandsOverOnTheNextAsk(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if strings.Contains(dir, "td-abc123") || !strings.Contains(dir, "compact") {
		t.Errorf("directive = %q, want the compacting answer, not the task", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("state.Task = %q, want unclaimed until the compaction lands", st.Task)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "dvalin" {
		t.Errorf("compacted = %v, want exactly one Compact(dvalin) fired", deps.compacted)
	}

	// The queued /compact has landed: the next ask reads a fresh figure under the threshold.
	deps.ctxTokens = 2_000
	dir, err = e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-abc123") {
		t.Errorf("directive = %q, want the open task claimed now the compaction has landed", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123", st.Task)
	}
	if len(deps.compacted) != 1 {
		t.Errorf("compacted = %v, want no second fire once it has landed", deps.compacted)
	}
}

// TestAPrematureReAskWaitsRatherThanRefiresOrAssigns: an ask that arrives before the queued
// compaction lands (the agent should not have asked again — DirCompacting says so — but this is
// the guard for when one does) must neither stack a second Compact behind the first nor fall
// through and assign into the context compaction was fired to avoid.
func TestAPrematureReAskWaitsRatherThanRefiresOrAssigns(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)

	if _, err := e.AgentDirective(context.Background(), "repo", "dvalin"); err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	// Reading is unchanged — the queued /compact has not run yet — but a second ask comes in anyway.
	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if strings.Contains(dir, "td-abc123") {
		t.Errorf("directive = %q, want the wait answer, not the task, while still stale", dir)
	}
	if len(deps.compacted) != 1 {
		t.Errorf("compacted = %v, want still exactly one — a stale re-ask must not stack a second fire", deps.compacted)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("state.Task = %q, want unclaimed while the reading is still stale", st.Task)
	}
}

// TestAnUnreadableSampleClearsPendingSoCompactionCanFireAgain isolates the ok=false path from the
// ordinary under-threshold one, since only the old code's early return skipped the clear — going
// straight from the unreadable sample back over the threshold means the ok=false step is the ONLY
// thing that could have cleared the flag. The old code left it true (Compact skipped, compacted
// stays at 1); the fix clears it there too (a second fire, compacted reaches 2).
func TestAnUnreadableSampleClearsPendingSoCompactionCanFireAgain(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := New(nil, deps)

	if dir, acted, err := e.compactOrWait("repo", "dvalin"); err != nil || !acted || dir != DirCompacting {
		t.Fatalf("compactOrWait = (%q, %v, %v), want the first fire", dir, acted, err)
	}
	if len(deps.compacted) != 1 {
		t.Fatalf("compacted = %v, want exactly one fire", deps.compacted)
	}

	deps.ctxOK = false // the transcript rotating to the fresh one /compact just wrote
	if _, acted, err := e.compactOrWait("repo", "dvalin"); err != nil || acted {
		t.Fatalf("compactOrWait on an unreadable sample = (%v, %v), want acted=false", acted, err)
	}

	// Straight back over the threshold — no under-threshold reading in between to clear the flag by
	// the pre-existing branch instead, which would mask the ok=false path under test.
	deps.ctxOK = true
	if dir, acted, err := e.compactOrWait("repo", "dvalin"); err != nil || !acted || dir != DirCompacting {
		t.Fatalf("compactOrWait = (%q, %v, %v), want a second fire — the unreadable sample must have cleared the flag", dir, acted, err)
	}
	if len(deps.compacted) != 2 {
		t.Errorf("compacted = %v, want exactly two fires — the second must not have been skipped", deps.compacted)
	}
}

// TestCompactionFiresAgainAfterLanding is the ordinary case the above isolates from: once a fired
// compaction lands (a low reading), a later rise back over the threshold fires a genuine second
// Compact rather than treating the agent as permanently done compacting.
func TestCompactionFiresAgainAfterLanding(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := New(nil, deps)

	if _, acted, err := e.compactOrWait("repo", "dvalin"); err != nil || !acted {
		t.Fatalf("compactOrWait = (%v, %v), want the first fire", acted, err)
	}

	deps.ctxTokens = 2_000 // landed
	if _, acted, err := e.compactOrWait("repo", "dvalin"); err != nil || acted {
		t.Fatalf("compactOrWait once landed = (%v, %v), want acted=false", acted, err)
	}

	deps.ctxTokens = 80_000 // due again
	if dir, acted, err := e.compactOrWait("repo", "dvalin"); err != nil || !acted || dir != DirCompacting {
		t.Fatalf("compactOrWait = (%q, %v, %v), want a second fire now it is due again", dir, acted, err)
	}
	if len(deps.compacted) != 2 {
		t.Errorf("compacted = %v, want exactly two fires", deps.compacted)
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

// TestBetweenSubtasksEndsThePassThenHandsOverOnTheNextAsk: the same rule claimNext follows, for a
// worker already holding a feature and about to be handed its next subtask (-> claimNextSubtask).
func TestBetweenSubtasksEndsThePassThenHandsOverOnTheNextAsk(t *testing.T) {
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
	if strings.Contains(dir, "td-next") || !strings.Contains(dir, "compact") {
		t.Errorf("directive = %q, want the compacting answer, not the subtask", dir)
	}
	if held, _ := ps.GetState(agent); held.Task != "" {
		t.Errorf("state.Task = %q, want unclaimed until the compaction lands", held.Task)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != agent {
		t.Errorf("compacted = %v, want exactly one Compact(%s) fired", deps.compacted, agent)
	}

	deps.ctxTokens = 2_000
	dir, err = e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "td-next") {
		t.Errorf("directive = %q, want the next subtask claimed now the compaction has landed", dir)
	}
	if held, _ := ps.GetState(agent); held.Task != "td-next" {
		t.Errorf("state.Task = %q, want td-next", held.Task)
	}
}

// TestReviewerFillPastTheCompactionThresholdEndsThePassThenHandsOverOnTheNextAsk: the same rule, for
// a reviewer about to be handed a new PR rather than a worker about to be handed a new task.
func TestReviewerFillPastTheCompactionThresholdEndsThePassThenHandsOverOnTheNextAsk(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := reviewerWithUnclaimedReview(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if strings.Contains(dir, "pr-1") || !strings.Contains(dir, "compact") {
		t.Errorf("directive = %q, want the compacting answer, not the review", dir)
	}
	if held, _ := ps.ReviewingPR("rune"); held != "" {
		t.Errorf("ReviewingPR = %q, want unclaimed until the compaction lands", held)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "rune" {
		t.Errorf("compacted = %v, want exactly one Compact(rune) fired", deps.compacted)
	}

	deps.ctxTokens = 2_000
	dir, err = e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "pr-1") {
		t.Errorf("directive = %q, want the review claimed now the compaction has landed", dir)
	}
	if held, _ := ps.ReviewingPR("rune"); held != "pr-1" {
		t.Errorf("ReviewingPR = %q, want pr-1", held)
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
