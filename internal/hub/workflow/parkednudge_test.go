package workflow

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// quietWorkerHoldingWork seeds a live worker holding a task with its screen gone still — the shape
// that produced "nudge stalled on os-8ea68f — idle for 6m0s".
func quietWorkerHoldingWork(t *testing.T, deps *stubDeps) (*Engine, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	deps.root, deps.alive = root, true
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "sd-1", Branch: "sd-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	return New(st, deps), ps
}

// TestAnOrdinaryStalledAgentIsStillNudged is the control, and it comes first: the exemption must not
// cost the nudge its purpose. Without this, a change that simply never nudged would pass every test
// below it.
func TestAnOrdinaryStalledAgentIsStillNudged(t *testing.T) {
	e, _ := quietWorkerHoldingWork(t, &stubDeps{})
	if !e.NudgeStalled("proj", "dvalin", "idle", 6*time.Minute) {
		t.Error("an agent that has genuinely gone quiet on held work should be nudged")
	}
}

// TestAContextFullAgentIsNotNudged: DirFull tells it "do not ask again, just wait". It obeyed, went
// quiet, and was then prodded every minute for obeying — the hub complaining about the one state it
// deliberately put the agent in.
func TestAContextFullAgentIsNotNudged(t *testing.T) {
	e, _ := quietWorkerHoldingWork(t, &stubDeps{ctxTokens: 900_000, ctxWindow: 1_000_000, ctxOK: true})
	if e.NudgeStalled("proj", "dvalin", "idle", 6*time.Minute) {
		t.Error("a full agent was nudged for waiting as it was told to")
	}
}

// TestARetiredAgentIsNotNudged is the same state reached deliberately by a human, and it must be
// exempt for the same reason — the agent has been wound down, not lost.
func TestARetiredAgentIsNotNudged(t *testing.T) {
	e, ps := quietWorkerHoldingWork(t, &stubDeps{})
	a, _, _ := ps.GetAgent("dvalin")
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if e.NudgeStalled("proj", "dvalin", "idle", 6*time.Minute) {
		t.Error("a retired agent was nudged for waiting as it was told to")
	}
}

// TestAGatedFeatureWorkerIsNotNudged: ReplyFeatureGated promises a push once the user rules, so the
// stall watchdog re-serving the same wait every dwell in the meantime is exactly the noise a review
// of this feature found — assignPendingSubtask (task.go) is what actually resolves it.
func TestAGatedFeatureWorkerIsNotNudged(t *testing.T) {
	deps := &stubDeps{}
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	deps.root, deps.alive = root, true
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "dain", Role: "worker", Workspace: ".worktrees/dain"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-EPIC", Title: "a feature", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-gated", Title: "gated", Status: "open", ParentID: "td-EPIC"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetApproval("td-gated", "pending", ""); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dain", Container: "td-EPIC", Branch: "td-EPIC", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	e := New(st, deps)

	if e.NudgeStalled("proj", "dain", "idle", 6*time.Minute) {
		t.Error("a feature worker waiting on a gate was nudged for waiting as it was told to")
	}
}

// TestACutOffTurnIsStillRetried: the api-error retry is not a complaint about idling, it asks a turn
// that stopped mid-sentence to resume. A parked agent still deserves that, since nothing about being
// wound down means its last turn should be left broken.
func TestACutOffTurnIsStillRetried(t *testing.T) {
	e, ps := quietWorkerHoldingWork(t, &stubDeps{})
	a, _, _ := ps.GetAgent("dvalin")
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if !e.NudgeStalled("proj", "dvalin", "api-error", 6*time.Minute) {
		t.Error("a cut-off turn should still be retried, whatever the agent's standing")
	}
}
