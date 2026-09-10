package fleet

import (
	"context"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// reviewerWithUnclaimedReview seeds a repo with one open, unclaimed review and a free reviewer —
// ReviewDirective's own claiming path, before any prep gate.
func reviewerWithUnclaimedReview(t *testing.T, deps *stubDeps) (*Engine, *store.ProjectStore) {
	t.Helper()
	root := t.TempDir()
	deps.Root = root
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
	return newEngine(t, st, deps), ps
}

// TestAnEmptyWorkerSessionIsHandedWorkDirectly is the control: a session seen to hold nothing has
// nothing to discard, so the claimed directive answers this ask directly.
func TestAnEmptyWorkerSessionIsHandedWorkDirectly(t *testing.T) {
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
	if len(deps.Cleared) != 0 {
		t.Errorf("cleared = %v, want none — an empty session has nothing to discard", deps.Cleared)
	}
}

// TestBetweenSubtasksClearsThenDeliversTheDirective: claimNextSubtask follows the same rule
// ClaimNext does, for a worker already holding a feature and about to be handed its next subtask —
// this ask answers with the clearing state once the clear fires, and the real subtask directive is queued
// behind /clear instead.
func TestBetweenSubtasksClearsThenDeliversTheDirective(t *testing.T) {
	const agent = "dain"
	root, _ := flowtest.WorkRepo(t, agent, "td-EPIC")
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
	// Between subtasks: the feature is held, but nothing is currently Assigned within it.
	if err := ps.SetState(store.AgentState{Agent: agent, Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatalf("set state: %v", err)
	}

	deps := &stubDeps{Root: root, CtxTokens: 80_000, CtxWindow: 200_000, CtxOK: true}
	e := newEngine(t, st, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", agent)
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if strings.Contains(dir, "One moment") || prompts.Deferring(dir) {
		t.Errorf("directive = %q, want the work itself — the ask settles before it reports, so a "+
			"placeholder is no longer an answer anybody has to be given", dir)
	}
	if held, _ := ps.GetState(agent); held.Task != "td-next" {
		t.Errorf("state.Task = %q, want td-next — the claim holds regardless of what this ask answers", held.Task)
	}
	if len(deps.Cleared) != 1 || deps.Cleared[0] != agent {
		t.Errorf("cleared = %v, want exactly one Clear(%s) fired", deps.Cleared, agent)
	}
	if len(deps.InjectedText) != 1 || !strings.Contains(deps.InjectedText[0], "td-next") {
		t.Errorf("injectedText = %v, want the next-subtask directive delivered after the clear", deps.InjectedText)
	}
}

// TestAnEmptyReviewerSessionIsHandedTheReviewDirectly is the one case that skips the clear: a session
// seen to hold nothing has nothing to discard, and /clear there would leave awaitCleared waiting out
// its cap for a drop that cannot come (-> SetModel's own guard).
func TestAnEmptyReviewerSessionIsHandedTheReviewDirectly(t *testing.T) {
	deps := &stubDeps{CtxTokens: 0, CtxWindow: 200_000, CtxOK: true}
	e, ps := reviewerWithUnclaimedReview(t, deps)

	dir, err := e.AgentDirective(context.Background(), "repo", "rune")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "pr-1") {
		t.Errorf("directive = %q, want the open review claimed and answered directly", dir)
	}
	if held, _ := ps.ReviewingPR("rune"); held != "pr-1" {
		t.Errorf("ReviewingPR = %q, want pr-1", held)
	}
	if len(deps.Cleared) != 0 {
		t.Errorf("cleared = %v, want none — an empty session has nothing to discard", deps.Cleared)
	}
}
