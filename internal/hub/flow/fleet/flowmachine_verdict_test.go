package fleet

import (
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"io"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// verdictFixture is one reviewer holding an Assigned review of one open PR.
func verdictFixture(t *testing.T) (*Engine, *store.ProjectStore, *stubDeps) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutPR(store.PR{ID: "pr-a", Task: "td-a", Agent: "bombur", Branch: "pr-a", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	// The AUTHOR is on the roster too: answering a rejection is its own map's next round, and a
	// machine with no agent row behind the name settles nothing at all.
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: "bombur"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "td-a", Branch: "pr-a", Phase: "submitted"},
		store.ReasonAdvanced, "test setup"); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{Root: t.TempDir(), Alive: true}
	e := newEngine(t, st, deps)
	flowtest.AssignReviewer(t, ps, "pr-a", "fili")
	return e, ps, deps
}

// TestApproveWakesTheReviewer is the sd-98fa96 fix, now via sd-a19ef9's clear: a verdict must not be
// where a reviewer's loop ends. Firing the clear is what wakes fili — the kickoff behind it gives the
// reviewer a reason to run `sindri` again, rather than leaving it on "awaiting human merge".
func TestApproveWakesTheReviewer(t *testing.T) {
	e, _, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.prAct().CmdApprove(c, []string{"pr-a"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdApprove: code=%d err=%v", code, err)
	}
	if len(deps.Injected) == 0 || deps.Injected[len(deps.Injected)-1] != "fili" {
		t.Fatalf("fili was not woken after its verdict: %v", deps.Injected)
	}
	for _, name := range deps.Cleared {
		if name == "fili" {
			t.Errorf("cleared = %v — waking a reviewer is a push; its own next job is what prepares it", deps.Cleared)
		}
	}
}

// TestARejectionClearsTheWorkerForItsNextRound is dain's case, which had no test and no clear: a
// rejection is a new ROUND on the same task, so it never reached the claim path's preparation and the
// worker reopened its own rejected reasoning on top of 642k of context. The verdict is the boundary.
func TestARejectionClearsTheWorkerForItsNextRound(t *testing.T) {
	e, _, deps := verdictFixture(t)
	deps.CtxOK, deps.CtxTokens, deps.CtxWindow = true, 642_000, 1_000_000
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.prAct().CmdReject(c, []string{"pr-a", "another", "pass"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdReject: code=%d err=%v", code, err)
	}
	// The verdict records the rejection; clearing for the next round is the AUTHOR's own map on its
	// way to answering the feedback (-> worker/refreshing).
	e.Look("repo", "bombur")
	if len(deps.Cleared) != 1 || deps.Cleared[0] != "bombur" {
		t.Fatalf("cleared = %v, want exactly one Clear(bombur) — the worker starts its round fresh", deps.Cleared)
	}
	// The feedback still has to reach the worker, and behind the clear rather than into what it wiped.
	// Not the LAST delivery — the reviewer's own wake follows this one (-> TestRejectWakesTheReviewer).
	var told bool
	for _, name := range deps.Injected {
		told = told || name == "bombur"
	}
	if !told {
		t.Errorf("injected = %v, want the rejection delivered to bombur after the clear", deps.Injected)
	}
}

// TestAnUnreadWorkerSessionIsNotClearedOnRejection: it takes a positive reading to fire, so a worker
// nobody has sampled — a just-launched one — is not handed a /clear that cannot land.
func TestAnUnreadWorkerSessionIsNotClearedOnRejection(t *testing.T) {
	e, _, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.prAct().CmdReject(c, []string{"pr-a", "another", "pass"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdReject: code=%d err=%v", code, err)
	}
	if len(deps.Cleared) != 0 {
		t.Errorf("cleared = %v, want none — nothing was recorded to discard", deps.Cleared)
	}
}

// TestRejectWakesTheReviewer is the same fix on the other verdict — both end a review.
func TestRejectWakesTheReviewer(t *testing.T) {
	e, _, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.prAct().CmdReject(c, []string{"pr-a", "not", "yet"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdReject: code=%d err=%v", code, err)
	}
	if len(deps.Injected) == 0 || deps.Injected[len(deps.Injected)-1] != "fili" {
		t.Fatalf("fili was not woken after its verdict: %v", deps.Injected)
	}
	for _, name := range deps.Cleared {
		if name == "fili" {
			t.Errorf("cleared = %v — waking a reviewer is a push; its own next job is what prepares it", deps.Cleared)
		}
	}
}
