package workflow

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// compactableWorker is idleWorkerWithOpenTask's twin with the open task omitted, for the cases that
// must show compaction is withheld with nothing to compact ahead of.
func compactableWorker(t *testing.T, deps *stubDeps) (*Engine, *store.ProjectStore) {
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
	if err := ps.SetState(store.AgentState{Agent: agent, Phase: "idle"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	return New(st, deps), ps
}

// TestFireDueCompactionsCompactsAWorkerWithAWaitingTask: fill past threshold AND an actual open task
// to prepare for — the sweep must fire.
func TestFireDueCompactionsCompactsAWorkerWithAWaitingTask(t *testing.T) {
	deps := &stubDeps{alive: true, ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, _ := idleWorkerWithOpenTask(t, deps)

	e.FireDueCompactions("repo")

	if len(deps.compacted) != 1 || deps.compacted[0] != "dvalin" {
		t.Errorf("compacted = %v, want exactly one Compact(dvalin)", deps.compacted)
	}
}

// TestFireDueCompactionsLeavesAnIdleWorkerWithNothingQueued: fill past threshold but no task waits —
// compacting here would be upkeep for whoever happens to be idle, not prep for an assignment, so the
// sweep must leave it alone.
func TestFireDueCompactionsLeavesAnIdleWorkerWithNothingQueued(t *testing.T) {
	deps := &stubDeps{alive: true, ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, _ := compactableWorker(t, deps)

	e.FireDueCompactions("repo")

	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — nothing was queued to prepare for", deps.compacted)
	}
}

// TestFireDueCompactionsSkipsRetiredAndClearArmed: retirement exempts every automatic behaviour, and
// an armed clear wins outright — neither should see a Compact.
func TestFireDueCompactionsSkipsRetiredAndClearArmed(t *testing.T) {
	for _, c := range []struct {
		name   string
		mutate func(a store.Agent) store.Agent
	}{
		{"retired", func(a store.Agent) store.Agent { a.Retired = true; return a }},
		{"clear-armed", func(a store.Agent) store.Agent { a.ClearArmed = true; return a }},
	} {
		t.Run(c.name, func(t *testing.T) {
			deps := &stubDeps{alive: true, ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
			e, ps := idleWorkerWithOpenTask(t, deps)
			a, _, err := ps.GetAgent("dvalin")
			if err != nil {
				t.Fatal(err)
			}
			if err := ps.PutAgent(c.mutate(a)); err != nil {
				t.Fatal(err)
			}

			e.FireDueCompactions("repo")

			if len(deps.compacted) != 0 {
				t.Errorf("%s agent was compacted anyway: %v", c.name, deps.compacted)
			}
		})
	}
}

// TestFireDueCompactionsSkipsMidTask: never mid-task — the same boundary clear waits for.
func TestFireDueCompactionsSkipsMidTask(t *testing.T) {
	deps := &stubDeps{alive: true, ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := idleWorkerWithOpenTask(t, deps)
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-abc123", Phase: "working"}); err != nil {
		t.Fatal(err)
	}

	e.FireDueCompactions("repo")

	if len(deps.compacted) != 0 {
		t.Errorf("a worker mid-task was compacted anyway: %v", deps.compacted)
	}
}

// reviewerWithUnclaimedReview seeds a repo with one open, unclaimed review and a free reviewer —
// reviewDirective's own claiming path, before any fullness gate.
func reviewerWithUnclaimedReview(t *testing.T, deps *stubDeps) (*Engine, *store.ProjectStore) {
	t.Helper()
	root := t.TempDir()
	deps.root = root
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "rune", Role: "reviewer"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	// UnclaimedReview requires the PR itself to read "open" — the status a review request leaves it
	// at, so anyone free can be handed it (-> RequestReview).
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "wrk", Branch: "td-1", Status: "open"}); err != nil {
		t.Fatalf("put pr: %v", err)
	}
	if _, err := ps.AddReview("pr-1", "check it"); err != nil {
		t.Fatalf("add review: %v", err)
	}
	return New(st, deps), ps
}

// TestFireDueCompactionsCompactsAReviewerWithAWaitingReview: the same rule, for a reviewer about to
// be handed a new PR rather than a worker about to be handed a new task.
func TestFireDueCompactionsCompactsAReviewerWithAWaitingReview(t *testing.T) {
	deps := &stubDeps{alive: true, ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, _ := reviewerWithUnclaimedReview(t, deps)

	e.FireDueCompactions("repo")

	if len(deps.compacted) != 1 || deps.compacted[0] != "rune" {
		t.Errorf("compacted = %v, want exactly one Compact(rune)", deps.compacted)
	}
}

// TestFireDueCompactionsSkipsAReviewerMidReview: a reviewer holding an open review is not at a leaf
// boundary — compacting mid-review is exactly as disruptive as mid-task.
func TestFireDueCompactionsSkipsAReviewerMidReview(t *testing.T) {
	deps := &stubDeps{alive: true, ctxTokens: 80_000, ctxWindow: 200_000, ctxOK: true, compactThreshold: 75_000}
	e, ps := reviewerWithUnclaimedReview(t, deps)
	var id int64
	var prID string
	if _, err := ps.UnclaimedReview(&id, &prID); err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(id, "rune"); err != nil {
		t.Fatal(err)
	}

	e.FireDueCompactions("repo")

	if len(deps.compacted) != 0 {
		t.Errorf("a reviewer mid-review was compacted anyway: %v", deps.compacted)
	}
}
