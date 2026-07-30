// package: hub / refwatch
// type:    logic (the tick behind reference-branch drift)
// job:     ask the workflow to re-check every project's reference branch on a slow
// loop, so a branch the user moves outside the hub is noticed rather than
// silently left under the agents working against it.
// limits:  just the cadence and lifecycle; the comparison and what agents are told
// live in workflow/reference.go. Agent liveness is the watchdog's, not this.
package hub

import (
	"log"
	"time"
)

// refInterval is slow on purpose: a reference branch moves when a human does something, not on a
// machine's schedule, and each pass rebases worktrees — cheap to be late, costly to thrash.
const refInterval = 30 * time.Second

// refwatch re-checks reference branches until stopped; one per hub, started by New.
type refwatch struct {
	h    *Hub
	stop chan struct{}
	done chan struct{}
	// lastErr is the failure already reported per project. A root that isn't a git repo fails
	// every pass, and on a 30s loop that buried the log — report a change, not a drip.
	lastErr map[string]string
}

// newRefwatch starts the loop. It must not block: New runs before Serve answers the socket.
func newRefwatch(h *Hub) *refwatch {
	r := &refwatch{h: h, stop: make(chan struct{}), done: make(chan struct{}), lastErr: map[string]string{}}
	go r.loop()
	return r
}

// loop sweeps on a ticker. The first sweep only records each tip, so nothing is reported as moved
// on the strength of a hub restart.
func (r *refwatch) loop() {
	defer close(r.done)
	t := time.NewTicker(refInterval)
	defer t.Stop()
	r.sweep()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
			r.sweep()
		}
	}
}

// sweep checks each registered project. A failure is logged and the others still run — one repo
// with a broken `reference:` must not blind the hub to the rest.
func (r *refwatch) sweep() {
	projects, err := r.h.store.Projects()
	if err != nil {
		log.Printf("hub: reference check: list projects: %v", err)
		return
	}
	for _, p := range projects {
		err := r.h.wf.SyncReference(p.Tag)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		// Log a failure once, and again only when it changes or clears — loud without a drip.
		if msg != r.lastErr[p.Tag] {
			if msg != "" {
				log.Printf("hub: reference check for %s (%s): %s", p.Tag, p.Path, msg)
			} else {
				log.Printf("hub: reference check for %s recovered", p.Tag)
			}
			r.lastErr[p.Tag] = msg
		}
	}
}

// close stops the loop and waits for the sweep in flight.
func (r *refwatch) close() {
	close(r.stop)
	<-r.done
}
