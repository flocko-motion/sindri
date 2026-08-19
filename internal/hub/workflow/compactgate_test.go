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

// TestFillPastTheCompactionThresholdClaimsThenQueuesTheDirectiveBehindCompact: the claim comes
// first — nothing here returns in the middle for the agent to be asked back from — but once
// compaction fires, THIS ask answers with DirPreparing, not the claimed directive: the real
// instruction is queued behind /compact instead, so the agent is never handed something to act on
// moments before the compaction that would cut it off.
func TestFillPastTheCompactionThresholdClaimsThenQueuesTheDirectiveBehindCompact(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claimed text must not be handed over directly", dir)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "td-abc123" {
		t.Errorf("state.Task = %q, want td-abc123 — the claim holds regardless of what this ask answers", st.Task)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "dvalin" {
		t.Errorf("compacted = %v, want exactly one Compact(dvalin) fired", deps.compacted)
	}
	if len(deps.compactedWith) != 1 || !strings.Contains(deps.compactedWith[0], "td-abc123") {
		t.Errorf("compactedWith = %v, want the claimed directive queued behind /compact", deps.compactedWith)
	}
}

// TestFillUnderTheCompactionThresholdIsHandedWork is the control: an ordinary claim under threshold
// never fires a compact at all, and the claimed directive answers this ask directly.
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

// TestAnUnreadableSampleDoesNotStopANextFire: compactIfDue keeps no memory of its own prior fire —
// an unreadable sample in between (the transcript rotating to the fresh one /compact just wrote)
// reads as simply not due, and the very next due reading fires again regardless.
func TestAnUnreadableSampleDoesNotStopANextFire(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := New(nil, deps)

	if _, err := e.compactIfDue("repo", "dvalin", "dir"); err != nil {
		t.Fatalf("compactIfDue: %v", err)
	}
	if len(deps.compacted) != 1 {
		t.Fatalf("compacted = %v, want exactly one fire", deps.compacted)
	}

	deps.ctxOK = false // the transcript rotating to the fresh one /compact just wrote
	if _, err := e.compactIfDue("repo", "dvalin", "dir"); err != nil {
		t.Fatalf("compactIfDue on an unreadable sample: %v", err)
	}
	if len(deps.compacted) != 1 {
		t.Fatalf("compacted = %v, want still exactly one — an unreadable sample must not fire again", deps.compacted)
	}

	deps.ctxOK = true
	if _, err := e.compactIfDue("repo", "dvalin", "dir"); err != nil {
		t.Fatalf("compactIfDue: %v", err)
	}
	if len(deps.compacted) != 2 {
		t.Errorf("compacted = %v, want a second fire — an unreadable sample in between is not remembered against it", deps.compacted)
	}
}

// TestCompactionFiresAgainAfterLanding is the ordinary case the above isolates from: once a fired
// compaction lands (a low reading), a later rise back over the threshold fires a genuine second
// Compact rather than treating the agent as permanently done compacting.
func TestCompactionFiresAgainAfterLanding(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e := New(nil, deps)

	if _, err := e.compactIfDue("repo", "dvalin", "dir"); err != nil {
		t.Fatalf("compactIfDue: %v", err)
	}

	deps.ctxTokens = 2_000 // landed
	if _, err := e.compactIfDue("repo", "dvalin", "dir"); err != nil {
		t.Fatalf("compactIfDue once landed: %v", err)
	}
	if len(deps.compacted) != 1 {
		t.Fatalf("compacted = %v, want still exactly one — landed fill is not due", deps.compacted)
	}

	deps.ctxTokens = 80_000 // due again
	if _, err := e.compactIfDue("repo", "dvalin", "dir"); err != nil {
		t.Fatalf("compactIfDue: %v", err)
	}
	if len(deps.compacted) != 2 {
		t.Errorf("compacted = %v, want exactly two fires", deps.compacted)
	}
}

// TestClearWinsOverCompaction is rule 1 in the epic's order: an armed clear preempts compaction
// outright, even when fill is also past the compaction threshold — a summary made first would be
// discarded seconds later by the wipe the user asked for by hand. claimNext's own clearArmed check
// runs before it ever looks for a compaction to fire, so nothing here reaches compactIfDue at all.
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

	fired, err := e.fireClearIfArmed("repo", "dvalin")
	if err != nil || !fired {
		t.Fatalf("fireClearIfArmed = (%v, %v), want fired", fired, err)
	}
	if len(deps.cleared) != 1 || deps.cleared[0] != "dvalin" {
		t.Errorf("cleared = %v, want exactly one FireClear(dvalin)", deps.cleared)
	}

	if _, claimed, err := e.claimNext("repo", "dvalin"); err != nil || claimed {
		t.Errorf("claimNext = (claimed=%v, err=%v), want nothing claimed this pass", claimed, err)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — the clear preempts it outright", deps.compacted)
	}
	if st, _ := ps.GetState("dvalin"); st.Task != "" {
		t.Errorf("a clear-armed worker was handed %q anyway", st.Task)
	}
}

// TestRetiredIsExemptFromCompaction: a retired worker is exempt from every automatic behaviour, not
// just new task assignment — it must read "retired", not fire a compaction.
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

// TestBetweenSubtasksClaimsThenQueuesTheDirectiveBehindCompact: claimNextSubtask follows the same
// rule claimNext does, for a worker already holding a feature and about to be handed its next
// subtask — this ask answers with DirPreparing once compaction fires, and the real subtask
// directive is queued behind /compact instead.
func TestBetweenSubtasksClaimsThenQueuesTheDirectiveBehindCompact(t *testing.T) {
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
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claimed text must not be handed over directly", dir)
	}
	if held, _ := ps.GetState(agent); held.Task != "td-next" {
		t.Errorf("state.Task = %q, want td-next — the claim holds regardless of what this ask answers", held.Task)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != agent {
		t.Errorf("compacted = %v, want exactly one Compact(%s) fired", deps.compacted, agent)
	}
	if len(deps.compactedWith) != 1 || !strings.Contains(deps.compactedWith[0], "td-next") {
		t.Errorf("compactedWith = %v, want the next-subtask directive queued behind /compact", deps.compactedWith)
	}
}

// TestReviewerFillPastTheCompactionThresholdClaimsThenQueuesTheDirectiveBehindCompact: the same
// rule, for a reviewer about to be handed a new PR rather than a worker about to be handed a new
// task.
func TestReviewerFillPastTheCompactionThresholdClaimsThenQueuesTheDirectiveBehindCompact(t *testing.T) {
	deps := &stubDeps{ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := reviewerWithUnclaimedReview(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if dir != DirPreparing {
		t.Errorf("directive = %q, want DirPreparing — the claimed text must not be handed over directly", dir)
	}
	if held, _ := ps.ReviewingPR("rune"); held != "pr-1" {
		t.Errorf("ReviewingPR = %q, want pr-1 — the claim holds regardless of what this ask answers", held)
	}
	if len(deps.compacted) != 1 || deps.compacted[0] != "rune" {
		t.Errorf("compacted = %v, want exactly one Compact(rune) fired", deps.compacted)
	}
	if len(deps.compactedWith) != 1 || !strings.Contains(deps.compactedWith[0], "pr-1") {
		t.Errorf("compactedWith = %v, want the review directive queued behind /compact", deps.compactedWith)
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
