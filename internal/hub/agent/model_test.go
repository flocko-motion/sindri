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
	if err := s.SetModel(t.Context(), "proj", "durin", "some-model-nobody-listed"); err == nil {
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

	if err := s.SetModel(t.Context(), "proj", "durin", "claude-opus-5"); err != nil {
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

	if err := s.SetModel(t.Context(), "proj", "durin", "claude-opus-5"); err != nil {
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
	// The clear takes the way a real one does — the transcript it leaves reports nothing — at the one
	// moment that proves the switch waited for it: while /clear is the only thing sent.
	f.afterSubmit = func() {
		if len(f.sent) == 1 && f.sent[0] == "/clear" {
			writeUsage(t, "proj", "durin", 0)
		}
	}

	if err := s.SetModel(t.Context(), "proj", "durin", "claude-opus-5"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	// Two sends and nothing else: the instruction that follows a switch is the caller's to deliver.
	want := []string{"/clear", "/model claude-opus-5"}
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
// there is nothing for /clear to do — the switch goes straight in.
func TestSetModelSkipsClearingAFreshSession(t *testing.T) {
	s, f := compactFixture(t)

	if err := s.SetModel(t.Context(), "proj", "durin", "claude-opus-5"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	if len(f.sent) != 1 || f.sent[0] != "/model claude-opus-5" {
		t.Errorf("sent = %v, want just the switch — nothing was recorded to clear", f.sent)
	}
	if len(f.removed) != 0 {
		t.Errorf("removed = %v, want none — a model change no longer relaunches the pod", f.removed)
	}
}
