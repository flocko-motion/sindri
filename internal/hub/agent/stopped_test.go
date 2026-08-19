package agent

import (
	"io"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestStopAgentPersistsStopped: a human's stop must be durable, so a hub restart between the call
// and the next launch still reads "stopped" (resumable) rather than blurring it back into "down"
// (crashed) — the whole reason the flag exists rather than living only in AgentStatus's argument.
func TestStopAgentPersistsStopped(t *testing.T) {
	s, _ := tellFixture(t, "eitri", idlePane)
	if err := s.StopAgent(t.Context(), "proj", "eitri"); err != nil {
		t.Fatal(err)
	}
	a, ok, err := s.store.For("proj").GetAgent("eitri")
	if err != nil || !ok {
		t.Fatalf("agent gone after stop: ok=%v err=%v", ok, err)
	}
	if !a.Stopped {
		t.Error("StopAgent must persist Stopped, or a hub restart reads a deliberate stop as a crash")
	}
}

// TestLaunchClearsStopped: asking a stopped agent to run again is no longer the human's deliberate
// down, whatever Launch itself goes on to do — checked against a fixture whose fake Check() fails
// on purpose, so this covers the flag independently of a launch actually succeeding.
func TestLaunchClearsStopped(t *testing.T) {
	s, _ := tellFixture(t, "eitri", idlePane)
	if err := s.store.For("proj").PutAgent(store.Agent{Name: "eitri", Role: "worker", Stopped: true}); err != nil {
		t.Fatal(err)
	}
	_ = s.Launch(t.Context(), "proj", "eitri", false, false, 0, 0, io.Discard)

	a, ok, err := s.store.For("proj").GetAgent("eitri")
	if err != nil || !ok {
		t.Fatalf("agent gone after launch: ok=%v err=%v", ok, err)
	}
	if a.Stopped {
		t.Error("Launch must clear Stopped as soon as it is asked to run again")
	}
}
