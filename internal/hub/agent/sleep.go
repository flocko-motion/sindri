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

	"github.com/flo-at/sindri/internal/hub/situation"
)

// IdleStopThreshold is how long a worker may hold nothing before the hub reclaims its pod. A first
// guess, named and alone because it will need tuning against real use.
const IdleStopThreshold = 30 * time.Minute

var idleSince struct {
	mu sync.Mutex
	at map[lcKey]time.Time
}

// HoldsNothing is no leaf task, no held feature, no review owed, no open escalation, nobody dialed
// in — the question the idle reclaim asks, where AtLeafBoundary is the narrower one a session reset
// asks. The rule itself is the surface's (-> situation.Situation.HoldsNothing); role is taken as an
// argument still because the sweeps have the roster row in hand and pass it.
func (s *Service) HoldsNothing(project, name, role string) (bool, error) {
	if role == "coauthor" {
		return false, nil
	}
	sit, err := s.sit.Of(project, name)
	if err != nil {
		return false, err
	}
	return sit.HoldsNothing(), nil
}

// FireIdleStops stops every non-retired worker that has held nothing past IdleStopThreshold.
// Idleness alone triggers it, never memory pressure: a stop preserves the session, so reclaiming
// costs only the next start's latency — no reason to wait for memory to be tight.
func (s *Service) FireIdleStops(ctx context.Context, project string) {
	roster, err := s.sit.Roster(project)
	if err != nil {
		return
	}
	now := time.Now()
	for _, sit := range roster {
		key := lcKey{project, sit.Name}
		// One question — may this pod be taken back — and the surface answers it, holdings, retirement
		// and an already-stopped pod together (-> situation.Surface.Reclaim).
		if sit.Allowed().Reclaim != "" {
			forgetIdleSince(key)
			continue
		}
		if !sit.Up {
			continue // nothing running to reclaim
		}
		since, due := idleSinceOrMark(key, now)
		if !due {
			continue
		}
		if err := s.stopAgent(ctx, project, sit.Name, fmt.Sprintf("idle for %s, reclaiming its pod", now.Sub(since).Round(time.Second))); err != nil {
			fmt.Fprintf(os.Stderr, "hub: idle-stopping %s: %v\n", sit.Name, err)
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
	roster, err := s.sit.Roster(project)
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

// wakeStoppedForRole wakes the first stopped agent of role in roster, unless a live one of that SAME
// role already holds nothing and would claim the work itself next poll. Who may be woken at all is
// the surface's (-> situation.Surface.Wake), which is where retirement is weighed.
func (s *Service) wakeStoppedForRole(ctx context.Context, project string, roster []situation.Situation, role, reason string) {
	toWake := ""
	for _, sit := range roster {
		if sit.Role != role || sit.Allowed().Wake != "" {
			continue
		}
		if sit.Stopped {
			if toWake == "" {
				toWake = sit.Name
			}
			continue
		}
		if sit.Up && sit.HoldsNothing() {
			return
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
