package agent

import (
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
	if err := s.SetModel(t.Context(), "proj", "durin", "some-model-nobody-listed", "next"); err == nil {
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

// TestSetModelStoresWithoutDisturbingAStoppedAgent: not running, so there is nothing live to
// retarget — the choice is just recorded for the next Launch to pick up.
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

	if err := s.SetModel(t.Context(), "proj", "durin", "claude-opus-5", "next"); err != nil {
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
// rather than through a first SetModel call, so this test isolates the repeat from the live
// switch a genuine change would trigger.
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

	if err := s.SetModel(t.Context(), "proj", "durin", "claude-opus-5", "next"); err != nil {
		t.Fatalf("SetModel to the value it already holds: %v", err)
	}
	if len(f.sent) != 0 || len(f.removed) != 0 {
		t.Errorf("an unchanged model disturbed the session — sent=%v removed=%v", f.sent, f.removed)
	}
}

// TestSetModelClearsBeforeItSwitches: the context belongs to the OLD model's reasoning, so it is
// discarded before the switch. The clear goes first and ALONE — /model opens a confirmation dialog,
// and a dialog swallows whatever is typed behind it, so a /clear sent after one is eaten and the
// switch runs against the very context it was meant to drop. The switch waits for the session to
// report itself empty, which is the clear having happened rather than time having passed.
func TestSetModelClearsBeforeItSwitches(t *testing.T) {
	s, f := compactFixture(t)
	writeUsage(t, "proj", "durin", 80_000) // a session with something in it, unlike a fresh one

	if err := s.SetModel(t.Context(), "proj", "durin", "claude-opus-5", "you hold td-abc123"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	// The clear is sent by the call itself; nothing may follow it until the session reads empty.
	if got := f.sent; len(got) != 1 || got[0] != "/clear" {
		t.Fatalf("sent = %v, want just the clear — the switch waits for it to take", got)
	}
	// It takes, the way a real /clear does: the transcript reports nothing.
	writeUsage(t, "proj", "durin", 0)
	waitForKickoff(s)

	want := []string{"/clear", "/model claude-opus-5", "you hold td-abc123"}
	got := f.sent
	if len(got) != len(want) {
		t.Fatalf("sent = %v, want %v in that order", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("sent[%d] = %q, want %q", i, got[i], w)
		}
	}
	if f.interrupts != 0 {
		t.Errorf("interrupts = %d, want none — queuing, not interrupting, is the whole point", f.interrupts)
	}
	if len(f.removed) != 0 {
		t.Errorf("removed = %v, want none — a model change no longer relaunches the pod", f.removed)
	}
	a, _, err := s.store.For("proj").GetAgent("durin")
	if err != nil {
		t.Fatal(err)
	}
	if a.Model != "claude-opus-5" {
		t.Errorf("Model = %q, want claude-opus-5", a.Model)
	}
}

// TestSetModelSkipsClearingAFreshSession: no recorded usage means nothing has been said yet, so
// there is nothing for /clear to do — the switch and the instruction still queue.
func TestSetModelSkipsClearingAFreshSession(t *testing.T) {
	s, f := compactFixture(t)

	if err := s.SetModel(t.Context(), "proj", "durin", "claude-opus-5", "you hold td-abc123"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	want := []string{"/model claude-opus-5", "you hold td-abc123"}
	if len(f.sent) != len(want) {
		t.Fatalf("sent = %v, want %v — no /clear, nothing was recorded to clear", f.sent, want)
	}
	for i, w := range want {
		if f.sent[i] != w {
			t.Errorf("sent[%d] = %q, want %q", i, f.sent[i], w)
		}
	}
	if len(f.removed) != 0 {
		t.Errorf("removed = %v, want none — a model change no longer relaunches the pod", f.removed)
	}
}
