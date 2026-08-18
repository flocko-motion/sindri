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
	if err := st.For("proj").SetState(store.AgentState{Agent: "durin", Phase: "idle"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeRuntime{pane: idlePane}
	container.Use(f)
	agentport.Use(claude.New())
	t.Cleanup(func() {
		container.UseDefault()
		agentport.Use(unreadablePane{})
	})
	forgetObservations()
	t.Cleanup(forgetObservations)
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

	if err := s.Compact("proj", "durin"); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	found := false
	for _, sent := range f.sent {
		if sent == "/compact" {
			found = true
		}
	}
	if !found {
		t.Errorf("/compact was never sent into the session; sent=%v", f.sent)
	}

	// The transcript compaction leaves behind is a fresh one — the memo must not serve the
	// pre-compaction figure back, the same bug that had a freshly cleared agent still read as full.
	writeUsage(t, "proj", "durin", 2_000)
	if got, _, _, ok := s.ContextUsage("proj", "durin"); !ok || got != 2_000 {
		t.Errorf("ContextUsage = (%d, %v), want the post-compaction reading (2000), not the stale one", got, ok)
	}
}

// TestCompactRefusesMidTask: never mid-task — the same boundary clear waits for, since compaction
// is exactly as disruptive to hold a leaf task through.
func TestCompactRefusesMidTask(t *testing.T) {
	s, f := compactFixture(t)
	writeUsage(t, "proj", "durin", 80_000)
	if err := s.store.For("proj").SetState(store.AgentState{Agent: "durin", Task: "td-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}

	if err := s.Compact("proj", "durin"); err == nil {
		t.Error("Compact should have refused a worker mid-task")
	}
	for _, sent := range f.sent {
		if sent == "/compact" {
			t.Error("a worker mid-task was compacted anyway")
		}
	}
}
