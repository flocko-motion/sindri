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
	e := New(st, deps)
	assignReviewer(t, ps, "pr-a", "fili")
	return e, ps, deps
}

// TestApproveWakesTheReviewer is the sd-98fa96 fix, now via sd-a19ef9's clear: a verdict must not be
// where a reviewer's loop ends. FireClear re-serves the directive itself once its /clear settles, so
// firing it is what wakes fili rather than leaving it on "awaiting human merge" with no reason to
// run `sindri` again.
func TestApproveWakesTheReviewer(t *testing.T) {
	e, _, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.CmdApprove(c, []string{"pr-a"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdApprove: code=%d err=%v", code, err)
	}
	if len(deps.cleared) == 0 || deps.cleared[len(deps.cleared)-1] != "fili" {
		t.Fatalf("no clear fired for fili after its verdict: %v", deps.cleared)
	}
}

// TestRejectWakesTheReviewer is the same fix on the other verdict — both end a review.
func TestRejectWakesTheReviewer(t *testing.T) {
	e, _, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.CmdReject(c, []string{"pr-a", "not", "yet"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdReject: code=%d err=%v", code, err)
	}
	if len(deps.cleared) == 0 || deps.cleared[len(deps.cleared)-1] != "fili" {
		t.Fatalf("no clear fired for fili after its verdict: %v", deps.cleared)
	}
}
