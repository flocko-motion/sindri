package harness

import (
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// sleepFixture wires a service over a fake tmux backend, with one running worker holding nothing.
func sleepFixture(t *testing.T) (*Service, *fakeRuntime) {
	t.Helper()
	_, st := newService(t)
	s := New(st, tellDeps{}, nil)
	if err := st.For("proj").PutAgent(store.Agent{Name: "durin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	flowtest.Place(t, st.For("proj"), store.AgentState{Agent: "durin", Phase: "idle"})
	f := &fakeRuntime{pane: idlePane}
	container.Use(f)
	agentport.Use(claude.New())
	t.Cleanup(func() {
		container.UseDefault()
		agentport.Use(unreadablePane{})
	})
	return s, f
}

// TestHoldsNothingChecksEveryKindOfWork: a task, a held feature, an owed review, or an open
// escalation each mean the worker is mid-something, not idle.
func TestHoldsNothingChecksEveryKindOfWork(t *testing.T) {
	s, _ := sleepFixture(t)
	if empty, err := s.HoldsNothing("proj", "durin", "worker"); err != nil || !empty {
		t.Fatalf("a fresh idle worker should hold nothing: empty=%v err=%v", empty, err)
	}
	ps := s.store.For("proj")
	for _, c := range []struct {
		name  string
		setup func() error
	}{
		{"task", func() error {
			flowtest.Place(t, ps, store.AgentState{Agent: "durin", Task: "td-1", Phase: "working"})
			return nil
		}},
		{"feature", func() error {
			flowtest.Place(t, ps, store.AgentState{Agent: "durin", Container: "sd-epic", Phase: "idle"})
			return nil
		}},
		{"escalation", func() error {
			flowtest.Place(t, ps, store.AgentState{Agent: "durin", Phase: "idle"})
			return ps.SetEscalation("durin", "which approach?")
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := c.setup(); err != nil {
				t.Fatal(err)
			}
			if empty, err := s.HoldsNothing("proj", "durin", "worker"); err != nil || empty {
				t.Errorf("holding %s should not read as holding nothing: empty=%v err=%v", c.name, empty, err)
			}
		})
	}
}

// TestHoldsNothingExcludesCoauthor: a coauthor's session is the user's own seat, never idle
// capacity — checked by role alone, before any state is even read.
func TestHoldsNothingExcludesCoauthor(t *testing.T) {
	s, _ := sleepFixture(t)
	if empty, err := s.HoldsNothing("proj", "durin", "coauthor"); err != nil || empty {
		t.Errorf("a coauthor must never read as holding nothing: empty=%v err=%v", empty, err)
	}
}
