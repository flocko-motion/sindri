// package: hub / ticks
// type:    assembly (every sweep the hub runs on a clock)
// job:     hold the hub's periodic work in ONE table — what each sweep is called, how often it
// runs, and whether it runs once at startup — and start and stop all of it together.
// limits:  the cadence and the lifecycle. What a sweep DOES belongs to whoever owns the subject;
// nothing here knows what a reference branch or a credential is.
package hub

import (
	"context"
	"time"
)

// A tick is one sweep on a clock. `atOnce` runs a pass before the first interval elapses, for work
// whose first pass establishes a baseline the later ones are read against.
type tick struct {
	name   string
	every  time.Duration
	atOnce bool
	sweep  func(context.Context)
}

// ticks runs every registered sweep until stopped. One goroutine each rather than one shared clock:
// they run at different rates, and a slow sweep must not delay a fast one.
type ticks struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// startTicks runs each sweep under a context derived from the hub's lifetime, so stopping cancels
// whatever a sweep has in flight rather than waiting it out.
func startTicks(base context.Context, list []tick) *ticks {
	ctx, cancel := context.WithCancel(base)
	t := &ticks{cancel: cancel, done: make(chan struct{}, len(list))}
	for _, entry := range list {
		go func(e tick) {
			defer func() { t.done <- struct{}{} }()
			if e.atOnce {
				e.sweep(ctx)
			}
			tk := time.NewTicker(e.every)
			defer tk.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tk.C:
					e.sweep(ctx)
				}
			}
		}(entry)
	}
	return t
}

// close stops every sweep and waits for the ones in flight.
func (t *ticks) close() {
	t.cancel()
	for range cap(t.done) {
		<-t.done
	}
}
