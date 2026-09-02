package workflow

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// verdictFixture is one reviewer holding an assigned review of one open PR.
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
	deps := &stubDeps{root: t.TempDir(), alive: true}
	e := newEngine(st, deps)
	assignReviewer(t, ps, "pr-a", "fili")
	return e, ps, deps
}

// TestApproveWakesTheReviewer is the sd-98fa96 fix, now via sd-a19ef9's clear: a verdict must not be
// where a reviewer's loop ends. Firing the clear is what wakes fili — the kickoff behind it gives the
// reviewer a reason to run `sindri` again, rather than leaving it on "awaiting human merge".
func TestApproveWakesTheReviewer(t *testing.T) {
	e, _, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.CmdApprove(c, []string{"pr-a"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdApprove: code=%d err=%v", code, err)
	}
	if len(deps.injected) == 0 || deps.injected[len(deps.injected)-1] != "fili" {
		t.Fatalf("fili was not woken after its verdict: %v", deps.injected)
	}
	for _, name := range deps.cleared {
		if name == "fili" {
			t.Errorf("cleared = %v — waking a reviewer is a push; its own next job is what prepares it", deps.cleared)
		}
	}
}

// TestARejectionClearsTheWorkerForItsNextRound is dain's case, which had no test and no clear: a
// rejection is a new ROUND on the same task, so it never reached the claim path's preparation and the
// worker reopened its own rejected reasoning on top of 642k of context. The verdict is the boundary.
func TestARejectionClearsTheWorkerForItsNextRound(t *testing.T) {
	e, _, deps := verdictFixture(t)
	deps.ctxOK, deps.ctxTokens, deps.ctxWindow = true, 642_000, 1_000_000
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.CmdReject(c, []string{"pr-a", "another", "pass"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdReject: code=%d err=%v", code, err)
	}
	if len(deps.cleared) != 1 || deps.cleared[0] != "bombur" {
		t.Fatalf("cleared = %v, want exactly one Clear(bombur) — the worker starts its round fresh", deps.cleared)
	}
	if len(deps.compacted) != 0 {
		t.Errorf("compacted = %v, want none — a round is prepared by the clear, never compaction", deps.compacted)
	}
	// The feedback still has to reach the worker, and behind the clear rather than into what it wiped.
	// Not the LAST delivery — the reviewer's own wake follows this one (-> TestRejectWakesTheReviewer).
	var told bool
	for _, name := range deps.injected {
		told = told || name == "bombur"
	}
	if !told {
		t.Errorf("injected = %v, want the rejection delivered to bombur after the clear", deps.injected)
	}
}

// TestAnUnreadWorkerSessionIsNotClearedOnRejection: it takes a positive reading to fire, so a worker
// nobody has sampled — a just-launched one — is not handed a /clear that cannot land.
func TestAnUnreadWorkerSessionIsNotClearedOnRejection(t *testing.T) {
	e, _, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.CmdReject(c, []string{"pr-a", "another", "pass"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdReject: code=%d err=%v", code, err)
	}
	if len(deps.cleared) != 0 {
		t.Errorf("cleared = %v, want none — nothing was recorded to discard", deps.cleared)
	}
}

// TestRejectWakesTheReviewer is the same fix on the other verdict — both end a review.
func TestRejectWakesTheReviewer(t *testing.T) {
	e, _, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.CmdReject(c, []string{"pr-a", "not", "yet"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdReject: code=%d err=%v", code, err)
	}
	if len(deps.injected) == 0 || deps.injected[len(deps.injected)-1] != "fili" {
		t.Fatalf("fili was not woken after its verdict: %v", deps.injected)
	}
	for _, name := range deps.cleared {
		if name == "fili" {
			t.Errorf("cleared = %v — waking a reviewer is a push; its own next job is what prepares it", deps.cleared)
		}
	}
}
