// package: hub/agent / sleep
// type:    logic (idle reclaim, and waking a stopped agent on demand)
// job:     stop an agent idle past IdleStopThreshold, reclaiming its pod; start a stopped one
// back up once its OWN kind of work is waiting and nothing else would take it. Off the
// hub's tick, like FireArmedClears: the fleet acted on unasked, not a request.
// limits:  the sweep and an in-memory "how long idle" record, lost on restart like
// stallwatch's own; claimable work is OpenLeaves/OpenContainers/UnclaimedReview, the gate's own queries.
package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
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
	// A PR still in flight is held work: the agent owns that task until it merges, and a rejection
	// hands it straight back. Missing this read a rejected author as an idle agent.
	if pr, _, aerr := ps.AwaitingPR(name); aerr != nil || pr != "" {
		return false, aerr
	}
	// store.Store's ReviewingPR, not ps's: a pooled reviewer's held review is never filed under
	// its own project, and reading it as "" here would let the sweep stop it mid-review.
	_, reviewing, err := s.store.ReviewingPR(project, name)
	if err != nil {
		return false, err
	}
	if reviewing != "" {
		return false, nil
	}
	if s.deps.AgentClients(project, name) > 0 {
		return false, nil // a human is dialed in; whatever they are doing, it is not the hub's to end
	}
	return true, nil
}

// FireIdleStops stops every non-retired worker that has held nothing past IdleStopThreshold.
// Idleness alone triggers it, never memory pressure: a stop preserves the session, so reclaiming
// costs only the next start's latency — no reason to wait for memory to be tight.
func (s *Service) FireIdleStops(ctx context.Context, project string) {
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
		if !s.deps.AgentUp(project, a.Name) {
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
		if err := s.stopAgent(ctx, project, a.Name, fmt.Sprintf("idle for %s, reclaiming its pod", now.Sub(since).Round(time.Second))); err != nil {
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

// FireIdleStarts wakes a stopped, non-retired agent when its OWN kind of work is waiting: a worker
// for an open task, a reviewer for an unclaimed PR — one role's queue must never wake the other's.
func (s *Service) FireIdleStarts(ctx context.Context, project string) {
	ps := s.store.For(project)
	packages, err := ps.OpenContainers()
	if err != nil {
		return
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		return
	}
	var reviewID int64
	var reviewPR string
	hasReview, err := ps.UnclaimedReview(&reviewID, &reviewPR)
	if err != nil {
		return
	}
	if len(packages) == 0 && len(leaves) == 0 && !hasReview {
		return // nothing waiting to wake anyone for
	}
	roster, err := ps.Roster()
	if err != nil {
		return
	}
	if len(packages) > 0 || len(leaves) > 0 {
		s.wakeStoppedForRole(ctx, project, roster, "worker", "work is waiting in this repo — starting")
	}
	if hasReview {
		s.wakeStoppedForRole(ctx, project, roster, "reviewer", "a review is waiting in this repo — starting")
	}
}

// wakeStoppedForRole wakes the first stopped, non-retired agent of role in roster, unless a live
// one of that SAME role already holds nothing and would claim the work itself next poll.
func (s *Service) wakeStoppedForRole(ctx context.Context, project string, roster []store.Agent, role, reason string) {
	toWake := ""
	for _, a := range roster {
		if a.Retired || a.Role != role {
			continue
		}
		if a.Stopped {
			if toWake == "" {
				toWake = a.Name
			}
			continue
		}
		if s.deps.AgentUp(project, a.Name) {
			if empty, _ := s.HoldsNothing(project, a.Name, a.Role); empty {
				return
			}
		}
	}
	if toWake == "" {
		return
	}
	_ = s.store.For(project).Log(toWake, "wake", reason)
	if err := s.Launch(ctx, project, toWake, false, false, 0, 0, io.Discard); err != nil {
		fmt.Fprintf(os.Stderr, "hub: waking %s for waiting work: %v\n", toWake, err)
	}
}
