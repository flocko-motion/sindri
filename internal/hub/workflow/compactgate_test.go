package workflow

import (
	"context"
	"strings"
	"testing"
)

// TestFillPastTheCompactionThresholdWithholdsTheNextTask: the assignment gate's whole point — a
// worker over the (much lower than ContextFullFraction) compaction bar is told to wait rather than
// handed the next task, since the actual firing is off-tick (-> agent.FireDueCompactions).
func TestFillPastTheCompactionThresholdWithholdsTheNextTask(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "compact") {
		t.Errorf("directive = %q, want it to say compaction is due", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("a worker over the compaction threshold was handed %q — it must stay unclaimed this cycle", st.Task)
	}
}

// TestFillUnderTheCompactionThresholdIsHandedWork is the control: an ordinary claim under threshold
// is untouched by the new gate.
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
}

// TestClearWinsOverCompaction is rule 1 in the epic's order: an armed clear preempts compaction
// outright, even when fill is also past the compaction threshold — a summary made first would be
// discarded seconds later by the wipe the user asked for by hand.
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
}

// TestCompactDueIgnoresAnUnknownWindow mirrors contextFull's own rule: a window nobody could
// resolve must never be treated as past a threshold computed from it.
func TestCompactDueIgnoresAnUnknownWindow(t *testing.T) {
	e := New(nil, &stubDeps{ctxTokens: 80_000, ctxWindow: 0, ctxOK: true, compactThreshold: 75_000})
	if _, due := e.compactDue("repo", "dvalin"); due {
		t.Error("a window of 0 must never read as past its own threshold")
	}
}

// TestReviewerFillPastTheCompactionThresholdWithholdsTheNextReview: the same gate, for a reviewer
// about to be handed a new PR rather than a worker about to be handed a new task.
func TestReviewerFillPastTheCompactionThresholdWithholdsTheNextReview(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := reviewerWithUnclaimedReview(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "compact") {
		t.Errorf("directive = %q, want it to say compaction is due", dir)
	}
	if held, _ := ps.ReviewingPR("rune"); held != "" {
		t.Errorf("a reviewer over the compaction threshold was handed %q — it must stay unclaimed this cycle", held)
	}
}

// TestReviewerFillUnderTheCompactionThresholdIsHandedAReview is the control: an ordinary claim under
// threshold is untouched by the new gate.
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
}
