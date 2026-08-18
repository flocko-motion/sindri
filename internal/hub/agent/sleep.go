// package: hub/agent / sleep
// type:    logic (idle reclaim, and waking a stopped worker on demand)
// job:     stop a worker idle past IdleStopThreshold, reclaiming its pod; start a stopped one
// back up once its repo has claimable work nothing else would take. Off the hub's tick,
// like FireDueCompactions/FireArmedClears: the fleet acted on unasked, not a request.
// limits:  the sweep and an in-memory "how long idle" record, lost on restart like
// stallwatch's own; claimable work is OpenLeaves/OpenContainers, the gate's own query.
package agent

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// IdleStopThreshold is how long a worker may hold nothing before the hub reclaims its pod. A first
// guess, named and alone because it will need tuning against real use.
const IdleStopThreshold = 30 * time.Minute

var idleSince struct {
	mu sync.Mutex
	at map[lcKey]time.Time
}

// HoldsNothing is no leaf task, no held feature, no review owed, no open escalation, nobody dialed
// in — stricter than AtLeafBoundary, which treats a held feature as fine to compact but not to
// reclaim the whole pod for. A coauthor is never this: its session is the user's own seat.
func (s *Service) HoldsNothing(project, name, role string) (bool, error) {
	if role == "coauthor" {
		return false, nil
	}
	ps := s.store.For(project)
	st, err := ps.GetState(name)
	if err != nil {
		return false, err
	}
	if st.Task != "" || st.Container != "" || st.Escalation != "" {
		return false, nil
	}
	reviewing, err := ps.ReviewingPR(name)
	if err != nil {
		return false, err
	}
	if reviewing != "" {
		return false, nil
	}
	if clients, err := s.Clients(project, name); err == nil && len(clients) > 0 {
		return false, nil // a human is dialed in; whatever they are doing, it is not the hub's to end
	}
	return true, nil
}

// FireIdleStops stops every non-retired worker that has held nothing past IdleStopThreshold.
// Idleness alone triggers it, never memory pressure: a stop preserves the session, so reclaiming
// costs only the next start's latency — no reason to wait for memory to be tight.
func (s *Service) FireIdleStops(project string) {
	roster, err := s.store.For(project).Roster()
	if err != nil {
		return
	}
	now := time.Now()
	for _, a := range roster {
		key := lcKey{project, a.Name}
		if a.Retired || a.Stopped {
			forgetIdleSince(key)
			continue
		}
		if !s.AgentAlive(project, a.Name) {
			continue // nothing running to reclaim
		}
		empty, err := s.HoldsNothing(project, a.Name, a.Role)
		if err != nil || !empty {
			forgetIdleSince(key)
			continue
		}
		since, due := idleSinceOrMark(key, now)
		if !due {
			continue
		}
		if err := s.stopAgent(project, a.Name, fmt.Sprintf("idle for %s, reclaiming its pod", now.Sub(since).Round(time.Second))); err != nil {
			fmt.Fprintf(os.Stderr, "hub: idle-stopping %s: %v\n", a.Name, err)
			continue
		}
		forgetIdleSince(key)
	}
}

// idleSinceOrMark records the first tick an agent was seen holding nothing, and reports whether
// that spell has now run past the threshold — a spell interrupted by new work and later idle again
// is correctly a new one, not a resumed one.
func idleSinceOrMark(key lcKey, now time.Time) (since time.Time, due bool) {
	idleSince.mu.Lock()
	defer idleSince.mu.Unlock()
	if idleSince.at == nil {
		idleSince.at = map[lcKey]time.Time{}
	}
	since, tracked := idleSince.at[key]
	if !tracked {
		idleSince.at[key] = now
		return now, false
	}
	return since, now.Sub(since) >= IdleStopThreshold
}

func forgetIdleSince(key lcKey) {
	idleSince.mu.Lock()
	delete(idleSince.at, key)
	idleSince.mu.Unlock()
}

// FireIdleStarts wakes one stopped, non-retired worker when the repo has claimable work and no
// live idle one would take it on its own next poll. Which worker: whichever the roster scan
// reaches first — no tier system yet to pick a better fit (-> sd-f76aea).
func (s *Service) FireIdleStarts(project string) {
	ps := s.store.For(project)
	packages, err := ps.OpenContainers()
	if err != nil {
		return
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		return
	}
	if len(packages) == 0 && len(leaves) == 0 {
		return // nothing waiting to wake anyone for
	}
	roster, err := ps.Roster()
	if err != nil {
		return
	}
	toWake := ""
	for _, a := range roster {
		if a.Retired || a.Role == "coauthor" {
			continue
		}
		if a.Stopped {
			if toWake == "" {
				toWake = a.Name
			}
			continue
		}
		if s.AgentAlive(project, a.Name) {
			if empty, _ := s.HoldsNothing(project, a.Name, a.Role); empty {
				return // already idle and alive: it claims this itself within one poll
			}
		}
	}
	if toWake == "" {
		return
	}
	_ = ps.Log(toWake, "wake", "work is waiting in this repo — starting")
	if err := s.Launch(project, toWake, false, false, 0, 0, io.Discard); err != nil {
		fmt.Fprintf(os.Stderr, "hub: waking %s for waiting work: %v\n", toWake, err)
	}
}
