package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// setLiveness writes an observation under watch.mu. The raw `h.watch.obs[key] = ...` pattern used
// elsewhere in this package predates statuswatch, but newHub(t) now drives it from the same real
// watchdog goroutine every hub test starts — an unsynchronized write here races that goroutine's own
// record() calls.
func setLiveness(h *Hub, project, name string, l liveness) {
	h.watch.mu.Lock()
	defer h.watch.mu.Unlock()
	h.watch.obs[agentKey{project, name}] = l
}

// The first sweep after an agent appears must establish a silent baseline: nothing here has ever
// seen it before, so its very first reading is not a "change" to log.
func TestStatuswatchNoLogOnFirstSweep(t *testing.T) {
	h := newHub(t)
	if _, err := h.agents.NewAgent(testProject, "brokkr", "worker", ""); err != nil {
		t.Fatal(err)
	}
	setLiveness(h, testProject, "brokkr", liveness{up: true})
	h.status.sweep()

	log, err := h.store.For(testProject).StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 0 {
		t.Errorf("a first-ever reading must not log as a change, got %+v", log)
	}
}

// A real change in the derived word — here, up flipping to down — must log exactly one row, once.
func TestStatuswatchLogsOnRealChange(t *testing.T) {
	h := newHub(t)
	if _, err := h.agents.NewAgent(testProject, "brokkr", "worker", ""); err != nil {
		t.Fatal(err)
	}
	setLiveness(h, testProject, "brokkr", liveness{up: true})
	h.status.sweep() // baseline

	setLiveness(h, testProject, "brokkr", liveness{up: false})
	h.status.sweep()

	log, err := h.store.For(testProject).StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 {
		t.Fatalf("want exactly one logged change, got %+v", log)
	}
	if log[0].Reason != string(store.ReasonStatus) {
		t.Errorf("want reason %q, got %q", store.ReasonStatus, log[0].Reason)
	}
}

// Sweeping repeatedly with nothing actually different must not log again — the whole point of
// diffing rather than recording every tick.
func TestStatuswatchSilentWhenUnchanged(t *testing.T) {
	h := newHub(t)
	if _, err := h.agents.NewAgent(testProject, "brokkr", "worker", ""); err != nil {
		t.Fatal(err)
	}
	setLiveness(h, testProject, "brokkr", liveness{up: true})
	h.status.sweep() // baseline
	h.status.sweep()
	h.status.sweep()

	log, err := h.store.For(testProject).StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 0 {
		t.Errorf("an unchanged word must not be logged again, got %+v", log)
	}
}

// The watchdog's own sweep must drive statuswatch too — the actual fix for the 3s-sampler blocker:
// a state that only ever holds for one watchdog beat must still be caught, which only a tail call
// inside watchdog.sweep (not a slower, separate ticker) can guarantee.
func TestWatchdogSweepDrivesStatuswatch(t *testing.T) {
	h := newHub(t)
	if _, err := h.agents.NewAgent(testProject, "brokkr", "worker", ""); err != nil {
		t.Fatal(err)
	}
	setLiveness(h, testProject, "brokkr", liveness{up: true})
	h.watch.sweep(false) // baseline, via the real watchdog sweep — not a direct h.status.sweep() call

	setLiveness(h, testProject, "brokkr", liveness{up: false})
	h.watch.sweep(false)

	log, err := h.store.For(testProject).StateLog("brokkr", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 {
		t.Fatalf("watchdog.sweep must drive the diff-check at its own tail, got %+v", log)
	}
}
