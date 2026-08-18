package agent

import (
	"strings"
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestSetModelRefusesAnUnknownModel: a model the backend's window table doesn't recognise is one
// whose fullness (and compaction threshold) the hub cannot judge — refused outright, never stored.
func TestSetModelRefusesAnUnknownModel(t *testing.T) {
	s, _ := compactFixture(t)
	if err := s.SetModel("proj", "durin", "some-model-nobody-listed"); err == nil {
		t.Fatal("SetModel accepted a model with no known window")
	}
	a, _, err := s.store.For("proj").GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	if a.Model != "" {
		t.Errorf("Model = %q, want unchanged after a refused choice", a.Model)
	}
}

// TestSetModelStoresWithoutDisturbingAStoppedAgent: not running, so there is nothing to compact —
// the choice is just recorded for the next Launch to pick up.
func TestSetModelStoresWithoutDisturbingAStoppedAgent(t *testing.T) {
	_, st := newService(t)
	s := New(st, tellDeps{}, nil)
	if err := st.For("proj").PutAgent(store.Agent{Name: "durin", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	container.UseDefault() // nothing running, as an unwired process finds it
	t.Cleanup(container.UseDefault)
	agentport.Use(claude.New()) // real ModelWindow, not the partial fakes other tests leave wired
	t.Cleanup(func() { agentport.Use(unreadablePane{}) })

	if err := s.SetModel("proj", "durin", "claude-opus-5"); err != nil {
		t.Fatalf("SetModel on a stopped agent: %v", err)
	}
	a, _, err := s.store.For("proj").GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	if a.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want claude-opus-5", a.Model)
	}
}

// TestSetModelToTheSameValueIsANoOp: nothing to disturb when nothing changes — seeded directly
// rather than through a first SetModel call, so this test isolates the repeat from the compact
// and relaunch a genuine change would trigger.
func TestSetModelToTheSameValueIsANoOp(t *testing.T) {
	s, f := compactFixture(t)
	a, _, err := s.store.For("proj").GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	a.Model = "claude-opus-5"
	if err := s.store.For("proj").PutAgent(a); err != nil {
		t.Fatal(err)
	}

	if err := s.SetModel("proj", "durin", "claude-opus-5"); err != nil {
		t.Fatalf("SetModel to the value it already holds: %v", err)
	}
	if len(f.sent) != 0 || len(f.removed) != 0 {
		t.Errorf("an unchanged model disturbed the session — sent=%v removed=%v", f.sent, f.removed)
	}
}

// TestSetModelCompactsThenRelaunchesARunningAgent: the session belongs to its old model and cannot
// cross, so a change compacts first — checked as the observable side effects up to the point Launch
// itself refuses (the fixture's fake Check() fails on purpose; see compactFixture/fakeRuntime).
func TestSetModelCompactsThenRelaunchesARunningAgent(t *testing.T) {
	s, f := compactFixture(t)
	writeUsage(t, "proj", "durin", 80_000) // a session with something in it, unlike a fresh one

	err := s.SetModel("proj", "durin", "claude-opus-5")
	if err == nil || !strings.Contains(err.Error(), "nothing to launch into") {
		t.Fatalf("SetModel = %v, want it to reach (and fail at) Launch's pre-flight", err)
	}

	found := false
	for _, sent := range f.sent {
		if sent == "/compact" {
			found = true
		}
	}
	if !found {
		t.Errorf("a running agent's model change never compacted first; sent=%v", f.sent)
	}
	// Unlike Compact's other callers, this one interrupts: the restart tears the pod down right
	// after, so the turn dying here costs nothing an unforced wait would have saved.
	if f.interrupts == 0 {
		t.Error("a model change never forced the pane idle before compacting")
	}
	if len(f.removed) == 0 {
		t.Error("the old container was never torn down on the way to relaunching")
	}
	a, _, err := s.store.For("proj").GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	if a.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want claude-opus-5 recorded even though the relaunch itself failed", a.Model)
	}
}

// TestSetModelSkipsCompactionOnAFreshSession: no recorded usage means nothing has been said yet, so
// there is nothing to carry across — firing Claude Code's own /compact into it would only land on
// "Not enough messages to compact." The relaunch still runs.
func TestSetModelSkipsCompactionOnAFreshSession(t *testing.T) {
	s, f := compactFixture(t)

	err := s.SetModel("proj", "durin", "claude-opus-5")
	if err == nil || !strings.Contains(err.Error(), "nothing to launch into") {
		t.Fatalf("SetModel = %v, want it to reach (and fail at) Launch's pre-flight", err)
	}

	for _, sent := range f.sent {
		if sent == "/compact" {
			t.Errorf("a fresh session with no recorded usage was compacted anyway; sent=%v", f.sent)
		}
	}
	if len(f.removed) == 0 {
		t.Error("the old container was never torn down on the way to relaunching")
	}
}
