// package: hub / statuswatch
// type:    logic (the diff-check behind the derived-status debug log)
// job:     recompute each agent's DERIVED status word — the same fold State() renders — and
// log a state_log row (-> store.ReasonStatus) only when it differs from the last one
// seen for that agent.
// limits:  no loop of its own — sweep is a tail call from watchdog.sweep, after its own
// goroutines finish (calling this from inside one would deadlock on w.mu).
package hub

import (
	"sync"

	"github.com/flo-at/sindri/internal/hub/store"
)

// statuswatch diffs the derived status word; one per hub, its sweep driven externally.
type statuswatch struct {
	h  *Hub
	mu sync.Mutex
	// last is the status word each agent was last seen at — in-memory only, so a hub restart starts a
	// fresh baseline rather than reporting a "change" from a word nobody here remembers seeing.
	last map[agentKey]string
}

// newStatuswatch builds the diff-check. No goroutine of its own — watchdog.sweep drives it.
func newStatuswatch(h *Hub) *statuswatch {
	return &statuswatch{h: h, last: map[agentKey]string{}}
}

// sweep recomputes every agent's status word and logs whichever changed. Entries for an agent no
// longer on the roster are dropped, so churn does not grow last for the hub's lifetime.
func (s *statuswatch) sweep() {
	agents, err := s.h.store.AllAgents()
	if err != nil {
		return
	}
	// One gather per project, as the board does: this runs over the whole fleet on a tick, and a
	// situation per agent would pay the claimable-pool query once per row (-> situation.Gatherer.Roster).
	stalled := map[agentKey]bool{}
	for _, tag := range projectsOf(agents) {
		roster, rerr := s.h.sit.Roster(tag)
		if rerr != nil {
			return
		}
		for _, sit := range roster {
			stalled[agentKey{tag, sit.Name}] = sit.Allowed().Stalled
		}
	}
	seen := make(map[agentKey]bool, len(agents))
	for _, a := range agents {
		st, _ := s.h.store.For(a.Project).GetState(a.Name)
		l, observed := s.h.watch.get(a.Project, a.Name)
		// peekStatusWord, not statusWord: this diff-check must not decide when a settled launch/stop
		// intent retires, or the very act of watching would change what it measures.
		status := s.h.peekStatusWord(a, st, l, observed, stalled[agentKey{a.Project, a.Name}])
		key := agentKey{a.Project, a.Name}
		seen[key] = true
		s.mu.Lock()
		prior, had := s.last[key]
		s.last[key] = status
		s.mu.Unlock()
		if !had || prior == status {
			continue
		}
		_ = s.h.store.For(a.Project).LogState(a.Name, store.ReasonStatus, prior+" -> "+status)
	}
	s.mu.Lock()
	for key := range s.last {
		if !seen[key] {
			delete(s.last, key)
		}
	}
	s.mu.Unlock()
}
