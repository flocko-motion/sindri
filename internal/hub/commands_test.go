package hub

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestStatusTreatsNoObservationAsProofOfLife: cmdStatus runs from inside the agent's own pod, so the
// caller IS proof of life. A just-launched agent has no observation until the watchdog's next probe
// beat, and reading that gap as "down" told a fresh agent it wasn't running — only a CONFIRMED-down
// reading (an observation with up=false) may say so.
func TestStatusTreatsNoObservationAsProofOfLife(t *testing.T) {
	h := newHub(t)
	// Not registered in the store: cmdStatus reads only the watchdog, and an unregistered agent is
	// one the background sweep never touches, so nothing here races this test's own recordings.
	c := registry.Caller{Project: testProject, Agent: "galar", Role: "worker"}

	// Never observed: the caller running this command is proof enough.
	var out bytes.Buffer
	if _, err := h.cmdStatus(c, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "running: true") {
		t.Errorf("an unobserved agent should read as running (it is asking), got %q", out.String())
	}

	// Observed and up: still running.
	h.watch.record(store.Agent{Project: testProject, Name: "galar"}, true, 0, seen("idle", "d1"))
	out.Reset()
	if _, err := h.cmdStatus(c, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "running: true") {
		t.Errorf("an observed, up agent should read as running, got %q", out.String())
	}

	// Observed and confirmed down: only this reads as not running.
	for i := 0; i <= downStrikes; i++ {
		h.watch.record(store.Agent{Project: testProject, Name: "galar"}, false, 0, agent.Observation{})
	}
	out.Reset()
	if _, err := h.cmdStatus(c, nil, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "running: false") {
		t.Errorf("a confirmed-down observation should read as not running, got %q", out.String())
	}
}
