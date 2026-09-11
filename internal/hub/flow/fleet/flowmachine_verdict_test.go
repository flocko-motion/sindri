package fleet

import (
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/reviewer"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"io"
	"path/filepath"
	"strings"
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
	flowtest.Place(t, ps, store.AgentState{Agent: "bombur", Task: "td-a", Branch: "pr-a", Phase: "submitted"})
	deps := &stubDeps{Root: t.TempDir()}
	e := newEngine(t, st, deps)
	flowtest.Reviewer(t, ps, "fili")
	flowtest.FileReview(t, ps, "pr-a")
	e.Look("repo", "fili") // its own map takes the waiting review, as the hub's beat would
	if held, _ := ps.ReviewingPR("fili"); held != "pr-a" {
		t.Fatalf("setup: fili should hold pr-a, got %q", held)
	}
	// The hand-over above said what it had to say. What these tests are about is what the VERDICT
	// then does, so the recorder starts empty from here.
	deps.Injected, deps.InjectedText, deps.Delivered, deps.Cleared = nil, nil, nil, nil
	return e, ps, deps
}

// TestApproveLeavesTheReviewerIdleAndQuiet: a verdict must not be where a reviewer's loop ends, and
// nothing here has to SAY so. It lands the reviewer on idle, which the machine watches, and the next
// pull request arrives as its own hand-over — where "run `sindri` for your next job" only asked the
// reviewer to ask for what the hub was already holding.
func TestApproveLeavesTheReviewerIdleAndQuiet(t *testing.T) {
	e, ps, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.prAct().CmdApprove(c, []string{"pr-a"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdApprove: code=%d err=%v", code, err)
	}
	// The verdict records itself and announces; the MACHINE is what reads a review nobody holds any
	// more and stands the reviewer down, which is a beat away in production and a look here.
	e.Look("repo", "fili")
	if st, err := ps.GetState("fili"); err != nil || st.Phase != reviewer.Idle {
		t.Fatalf("fili is on %q (err %v), want idle — that is what the machine picks up from", st.Phase, err)
	}
	for _, name := range deps.Injected {
		if name == "fili" {
			t.Errorf("injected = %v — a freed reviewer is told nothing until there is something to tell", deps.Injected)
		}
	}
	for _, name := range deps.Cleared {
		if name == "fili" {
			t.Errorf("cleared = %v — a verdict is not a reason to reset a session", deps.Cleared)
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
	// The FEEDBACK itself has to reach the worker, behind the clear rather than into what it wiped —
	// landing in worker/reworking is what says it, so nothing has to ask the worker to ask.
	var told bool
	for i, name := range deps.Injected {
		told = told || (name == "bombur" && strings.Contains(deps.InjectedText[i], "another pass"))
	}
	if !told {
		t.Errorf("injected = %v / %q, want the findings themselves delivered to bombur after the clear",
			deps.Injected, deps.InjectedText)
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

// TestRejectLeavesTheReviewerIdleAndQuiet is the same rule on the other verdict — both end a review.
// The AUTHOR hears about a rejection; the reviewer that gave it has nothing left to be told.
func TestRejectLeavesTheReviewerIdleAndQuiet(t *testing.T) {
	e, ps, deps := verdictFixture(t)
	c := registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}
	if code, err := e.prAct().CmdReject(c, []string{"pr-a", "not", "yet"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("CmdReject: code=%d err=%v", code, err)
	}
	e.Look("repo", "fili")
	if st, err := ps.GetState("fili"); err != nil || st.Phase != reviewer.Idle {
		t.Fatalf("fili is on %q (err %v), want idle", st.Phase, err)
	}
	for _, name := range deps.Injected {
		if name == "fili" {
			t.Errorf("injected = %v — the reviewer is freed, not instructed", deps.Injected)
		}
	}
}
