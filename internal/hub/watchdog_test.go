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
