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

// TestSetModelStoresWithoutDisturbingAStoppedAgent: not running, so there is nothing to clear —
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
// rather than through a first SetModel call, so this test isolates the repeat from the clear
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

// TestSetModelClearsThenRelaunchesARunningAgent: the agent holds nothing at a model change (its own
// boundary check runs exactly here) and the context belongs to the OLD model's reasoning, so a
// change clears first, not compacts — checked as the observable side effects up to the point Launch
// itself refuses (the fixture's fake Check() fails on purpose; see compactFixture/fakeRuntime).
func TestSetModelClearsThenRelaunchesARunningAgent(t *testing.T) {
	s, f := compactFixture(t)
	writeUsage(t, "proj", "durin", 80_000) // a session with something in it, unlike a fresh one

	err := s.SetModel("proj", "durin", "claude-opus-5")
	if err == nil || !strings.Contains(err.Error(), "nothing to launch into") {
		t.Fatalf("SetModel = %v, want it to reach (and fail at) Launch's pre-flight", err)
	}

	found := false
	for _, sent := range f.sent {
		if sent == "/clear" {
			found = true
		}
	}
	if !found {
		t.Errorf("a running agent's model change never cleared first; sent=%v", f.sent)
	}
	// FireClear interrupts on its own — a human-armed-equivalent act here, not the agent's own
	// request, and the restart tears the pod down right after regardless.
	if f.interrupts == 0 {
		t.Error("a model change never forced the pane idle before clearing")
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

// TestSetModelSkipsClearingAFreshSession: no recorded usage means nothing has been said yet, so
// there is nothing for /clear to do. The relaunch still runs.
func TestSetModelSkipsClearingAFreshSession(t *testing.T) {
	s, f := compactFixture(t)

	err := s.SetModel("proj", "durin", "claude-opus-5")
	if err == nil || !strings.Contains(err.Error(), "nothing to launch into") {
		t.Fatalf("SetModel = %v, want it to reach (and fail at) Launch's pre-flight", err)
	}

	for _, sent := range f.sent {
		if sent == "/clear" {
			t.Errorf("a fresh session with no recorded usage was cleared anyway; sent=%v", f.sent)
		}
	}
	if len(f.removed) == 0 {
		t.Error("the old container was never torn down on the way to relaunching")
	}
}
