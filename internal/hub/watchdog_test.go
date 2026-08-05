package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestOneLostProbeDoesNotFlipAnAgentDown is the bug the watchdog exists for. A probe that loses
// a race is not evidence: the readings before it said the agent was up, and one contended exec
// cannot outweigh them. Per-request probing could never do this — each request began with no
// history, so a single miss WAS the verdict, and a healthy agent flickered.
func TestOneLostProbeDoesNotFlipAnAgentDown(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "galar"}

	w.record(a, true, 1, "working", false) // a good reading first
	if l, _ := w.get("proj", "galar"); !l.up {
		t.Fatal("a successful probe must report up")
	}

	// Failures short of the threshold hold the previous state, dial-in count and runtime
	// included: a probe that could not read the agent must not blank what the last one did.
	for i := 1; i < downStrikes; i++ {
		w.record(a, false, 0, "", false)
		l, _ := w.get("proj", "galar")
		if !l.up {
			t.Errorf("strike %d of %d already reported down", i, downStrikes)
		}
		if l.clients != 1 || l.runtime != "working" {
			t.Errorf("strike %d blanked the last good detail: clients=%d runtime=%q", i, l.clients, l.runtime)
		}
	}

	// The threshold reached, it is no longer one bad reading but a pattern.
	w.record(a, false, 0, "", false)
	if l, _ := w.get("proj", "galar"); l.up {
		t.Errorf("%d consecutive failures should report down", downStrikes)
	}
}

// TestSuccessClearsStrikes: an agent that answers again starts from a clean slate, so an earlier
// rough patch cannot combine with a later one to declare a healthy agent down.
func TestSuccessClearsStrikes(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "galar"}

	w.record(a, true, 0, "idle", false)
	w.record(a, false, 0, "", false) // one strike
	w.record(a, true, 2, "working", false)
	if l, _ := w.get("proj", "galar"); l.strikes != 0 || l.clients != 2 || l.runtime != "working" {
		t.Errorf("a success must reset strikes and take the fresh reading, got %+v", l)
	}
	// From clean, it again takes the full threshold to go down.
	for i := 1; i < downStrikes; i++ {
		w.record(a, false, 0, "", false)
		if l, _ := w.get("proj", "galar"); !l.up {
			t.Errorf("strike %d after a success reported down too early", i)
		}
	}
}

// TestAbsentPodIsConclusive: no pod in the listing needs no corroboration — there is nothing to
// be racing, so waiting three sweeps would just show a stopped agent as running.
func TestAbsentPodIsConclusive(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "galar"}

	w.record(a, true, 0, "working", false)
	w.record(a, false, 0, "", true) // conclusive: the pod is gone
	if l, _ := w.get("proj", "galar"); l.up {
		t.Error("an agent with no pod is down at once, not after three strikes")
	}
}

// TestUnobservedAgentIsNotReportedUp: an agent the watchdog has never seen has no observation,
// and the board must treat that as "not known to be up" rather than inventing liveness.
func TestUnobservedAgentIsNotReportedUp(t *testing.T) {
	h := newHub(t)
	if l, ok := h.watch.get("proj", "never-seen"); ok || l.up {
		t.Errorf("an unobserved agent must report no observation, got %+v ok=%v", l, ok)
	}
}

// TestIdleDwellStartsAndClears: the dwell is what separates a thinking pause from a stall, so it
// must start when idle begins and reset the moment the agent does anything.
func TestIdleDwellStartsAndClears(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "dvalin"}

	w.record(a, true, 0, "working", false)
	if l, _ := w.get("proj", "dvalin"); !l.idleSince.IsZero() {
		t.Error("a working agent has no idle dwell")
	}

	w.record(a, true, 0, "idle", false)
	first, _ := w.get("proj", "dvalin")
	if first.idleSince.IsZero() {
		t.Fatal("going idle must start the dwell")
	}

	// Still idle: the clock keeps running from when it started, not from the latest probe.
	w.record(a, true, 0, "idle", false)
	again, _ := w.get("proj", "dvalin")
	if !again.idleSince.Equal(first.idleSince) {
		t.Errorf("a continuing idle spell must keep its start: %v then %v", first.idleSince, again.idleSince)
	}

	// Back to work: the spell is over, and a later stall is a new one.
	w.record(a, true, 0, "working", false)
	if l, _ := w.get("proj", "dvalin"); !l.idleSince.IsZero() {
		t.Error("working again must clear the dwell")
	}
}

// TestALostProbeDoesNotRestartTheDwell: a failed probe holds the previous runtime, so it must hold
// the dwell too. Restarting the clock on every miss would hide a stall for another full dwell each
// time a probe lost a race — and the agent that stalls is often the one whose pod is contended.
func TestALostProbeDoesNotRestartTheDwell(t *testing.T) {
	h := newHub(t)
	w := h.watch
	a := store.Agent{Project: "proj", Name: "dvalin"}

	w.record(a, true, 0, "idle", false)
	started, _ := w.get("proj", "dvalin")

	w.record(a, false, 0, "", false) // one lost probe, short of downStrikes
	held, _ := w.get("proj", "dvalin")
	if held.runtime != "idle" {
		t.Fatalf("a lost probe should hold the last runtime, got %q", held.runtime)
	}
	if !held.idleSince.Equal(started.idleSince) {
		t.Errorf("the dwell restarted on a lost probe: %v then %v", started.idleSince, held.idleSince)
	}
}
