package agent

import (
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// compactFixture wires a service over a fake tmux backend and the real transcript reader (so
// CompactDue/CompactionThreshold compute against an actual file, not a stub), with one agent
// registered and idle.
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

// TestCompactDueCrossesTheThreshold: no model in the transcript resolves to the default 200k
// window, whose threshold (CompactionThreshold(200_000)) is 75_000 — the same anchor the formula's
// own test pins independently in the claude package.
func TestCompactDueCrossesTheThreshold(t *testing.T) {
	s, _ := compactFixture(t)
	writeUsage(t, "proj", "durin", 80_000)
	if tokens, due := s.CompactDue("proj", "durin"); !due || tokens != 80_000 {
		t.Errorf("CompactDue = (%d, %v), want (80000, true) — past the default window's threshold", tokens, due)
	}

	s.ForgetContext("proj", "durin") // else the memo would still answer with the first reading
	writeUsage(t, "proj", "durin", 10_000)
	if _, due := s.CompactDue("proj", "durin"); due {
		t.Error("CompactDue = true for a fill well under the threshold")
	}
}

// TestFireDueCompactionsInjectsAndForgetsTheMemo runs the real thing end to end: a live pane to
// inject into, a transcript on disk to measure. It asks the same question the clear regression did
// — after firing, does the hub still answer from the pre-compaction figure? — and additionally
// checks the injection itself landed.
func TestFireDueCompactionsInjectsAndForgetsTheMemo(t *testing.T) {
	s, f := compactFixture(t)
	writeUsage(t, "proj", "durin", 80_000)
	if _, ok := s.CompactDue("proj", "durin"); !ok {
		t.Fatal("fixture not past its own threshold")
	}
	// Memoise the pre-compaction reading, the way a board render would.
	if got, _, _, ok := s.ContextUsage("proj", "durin"); !ok || got != 80_000 {
		t.Fatalf("ContextUsage = (%d, %v), want the pre-compaction reading", got, ok)
	}

	s.FireDueCompactions("proj")

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

// TestFireDueCompactionsSkipsRetiredAndClearArmed: retirement exempts every automatic behaviour, and
// an armed clear wins outright — neither should see a /compact.
func TestFireDueCompactionsSkipsRetiredAndClearArmed(t *testing.T) {
	for _, c := range []struct {
		name   string
		mutate func(a store.Agent) store.Agent
	}{
		{"retired", func(a store.Agent) store.Agent { a.Retired = true; return a }},
		{"clear-armed", func(a store.Agent) store.Agent { a.ClearArmed = true; return a }},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, f := compactFixture(t)
			writeUsage(t, "proj", "durin", 80_000)
			ps := s.store.For("proj")
			a, _, err := ps.GetAgent("durin")
			if err != nil {
				t.Fatal(err)
			}
			if err := ps.PutAgent(c.mutate(a)); err != nil {
				t.Fatal(err)
			}

			s.FireDueCompactions("proj")

			for _, sent := range f.sent {
				if sent == "/compact" {
					t.Errorf("%s agent was compacted anyway", c.name)
				}
			}
		})
	}
}

// TestFireDueCompactionsSkipsMidTask: never mid-task — the same boundary clear waits for, since
// compaction is exactly as disruptive to hold a leaf task through.
func TestFireDueCompactionsSkipsMidTask(t *testing.T) {
	s, f := compactFixture(t)
	writeUsage(t, "proj", "durin", 80_000)
	if err := s.store.For("proj").SetState(store.AgentState{Agent: "durin", Task: "td-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}

	s.FireDueCompactions("proj")

	for _, sent := range f.sent {
		if sent == "/compact" {
			t.Error("a worker mid-task was compacted anyway")
		}
	}
}
