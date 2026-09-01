package hub

import (
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/observe"
)

// seedReading seeds the watchdog's standing reading for an agent, the way a sweep would.
func seedReading(h *Hub, name string, l liveness) {
	h.watch.mu.Lock()
	defer h.watch.mu.Unlock()
	if l.seen.IsZero() {
		l.seen = time.Now() // record always stamps one; an unstamped reading is nothing having looked
	}
	h.watch.obs[agentKey{"proj", name}] = l
}

// TestOnlyASignedOutAgentIsRevived: a fresh token reaches a running process only through a restart —
// the old one is held in memory, and a message typed at a /login prompt is swallowed. So the hub
// restarts the agents that cannot come back on their own, and ONLY those: an agent working normally
// must not be interrupted because a token was refreshed under it, which is routine.
func TestOnlyASignedOutAgentIsRevived(t *testing.T) {
	h := newHub(t)
	cw := &credwatch{h: h}

	for _, tc := range []struct {
		name string
		l    liveness
		want bool
	}{
		{"balin", liveness{up: true, state: observe.SignedOut}, true},
		{"bombur", liveness{up: true, state: observe.Working}, false},
		{"dvalin", liveness{up: true, state: observe.AtPrompt}, false},
		// Down is not stuck at a prompt — there is no process holding a stale token, and a restart
		// here would start an agent the user stopped.
		{"nori", liveness{up: false, state: observe.SignedOut}, false},
	} {
		seedReading(h, tc.name, tc.l)
		if got := cw.stuckAtLogin("proj", tc.name); got != tc.want {
			t.Errorf("%s (up=%v runtime=%q): stuckAtLogin = %v, want %v", tc.name, tc.l.up, tc.l.state, got, tc.want)
		}
	}

	// An agent nothing has looked at yet supports no claim either way, so it is left alone.
	if cw.stuckAtLogin("proj", "unseen") {
		t.Error("an unobserved agent was treated as stuck at a login prompt")
	}
}
