package agent

import (
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// compactFixture wires a service over a fake tmux backend and the real transcript reader (so
// CompactionThreshold computes against an actual file, not a stub), with one agent registered and
// idle. Deciding WHEN to compact is workflow.Engine's (task/review awareness Compact itself does
// not have); this only exercises Compact's own mechanics.
func compactFixture(t *testing.T) (*Service, *fakeRuntime) {
	t.Helper()
	t.Setenv("SINDRI_HOME", t.TempDir()) // AgentHomeDir reads this, so the transcript is ours
	_, st := newService(t)
	s := New(st, tellDeps{}, nil)
	if err := st.For("proj").PutAgent(store.Agent{Name: "durin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if err := st.For("proj").SetState(store.AgentState{Agent: "durin", Phase: "idle"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	f := &fakeRuntime{pane: idlePane}
	container.Use(f)
	agentport.Use(claude.New())
	t.Cleanup(func() {
		container.UseDefault()
		agentport.Use(unreadablePane{})
	})
	s.ForgetContext("proj", "durin") // contextMemo is package-level; a prior test's reading must not leak in
	return s, f
}

// TestCompactInjectsAndForgetsTheMemo runs the real thing end to end: a live pane to inject into, a
// transcript on disk to measure. It asks the same question the clear regression did — after firing,
// does the hub still answer from the pre-compaction figure? — and additionally checks the injection
// itself landed.
func TestCompactInjectsAndForgetsTheMemo(t *testing.T) {
	s, f := compactFixture(t)
	writeUsage(t, "proj", "durin", 80_000)
	// Memoise the pre-compaction reading, the way a board render would.
	if got, _, _, ok := s.ContextUsage("proj", "durin"); !ok || got != 80_000 {
		t.Fatalf("ContextUsage = (%d, %v), want the pre-compaction reading", got, ok)
	}

	// The transcript compaction leaves behind is a fresh one, written the moment /compact is
	// submitted — which is also the evidence Compact blocks on before it answers.
	f.afterSubmit = func() { writeUsage(t, "proj", "durin", 2_000) }
	if err := s.Compact(t.Context(), "proj", "durin"); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	// Never interrupted: the turn Compact is called from is the agent's own in-flight directive
	// request, and an Escape here used to kill it before that reply landed (-> the regression this
	// fixes). Nothing rides along behind the /compact either — what follows is the caller's to send.
	if f.interrupts != 0 {
		t.Errorf("Compact must never interrupt — the in-flight turn is the one about to answer it, got %d escape(s)", f.interrupts)
	}
	if len(f.sent) != 1 || f.sent[0] != "/compact" {
		t.Errorf("sent = %v, want just [/compact] — the command carries no follow-up of its own", f.sent)
	}

	// The memo must not serve the pre-compaction figure back, the same bug that had a freshly
	// cleared agent still read as full.
	if got, _, _, ok := s.ContextUsage("proj", "durin"); !ok || got != 2_000 {
		t.Errorf("ContextUsage = (%d, %v), want the post-compaction reading (2000), not the stale one", got, ok)
	}
}

// TestCompactPerformsOnAJustClaimedAgent is the case the deleted assignment window existed for: the
// gate claims first, so by the time it prepares the agent the store reads mid-task. Compact no longer
// asks — whether this is a safe moment is the caller's, and this caller is inside its own assignment.
func TestCompactPerformsOnAJustClaimedAgent(t *testing.T) {
	s, f := compactFixture(t)
	writeUsage(t, "proj", "durin", 80_000)
	if err := s.store.For("proj").SetState(store.AgentState{Agent: "durin", Task: "td-1", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	f.afterSubmit = func() { writeUsage(t, "proj", "durin", 2_000) }

	if err := s.Compact(t.Context(), "proj", "durin"); err != nil {
		t.Fatalf("Compact on a just-claimed agent: %v", err)
	}
	if len(f.sent) != 1 || f.sent[0] != "/compact" {
		t.Errorf("sent = %v, want [/compact] — the preparation the gate asked for must have run", f.sent)
	}
}

// TestFakeInterruptDetectionIsNotVacuous is the anti-vacuity floor for f.interrupts: without one
// real call that sends Escape to prove the fake's matcher fires, TestCompactInjectsAndForgetsTheMemo's
// zero-count assertion would pass whether or not that matcher actually works.
func TestFakeInterruptDetectionIsNotVacuous(t *testing.T) {
	s, f := compactFixture(t)
	if err := s.Interrupt(t.Context(), "proj", "durin"); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if f.interrupts != 1 {
		t.Errorf("interrupts = %d, want 1 — the fake's escape matcher did not fire", f.interrupts)
	}
}
