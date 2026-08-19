// package: hub / runwatch
// type:    logic (the tick behind executing queued runs)
// job:     start the next queued run, one at a time across the whole fleet. The queue and
// execution itself live in workflow (-> NextQueuedRun, ExecuteRun); this is only the
// cadence and the fleet-wide "one at a time" gate.
// limits:  just the cadence; a run orphaned by a hub restart is reconciled at startup, not here
// (-> workflow.ReconcileRunningRuns).
package hub

import (
	"context"
	"sync"
	"time"
)

// runInterval only has to be short against the 15-minute cap: a run waiting behind a slot that
// just freed should not sit idle for long, but the queue need not be polled continuously either.
const runInterval = 5 * time.Second

// runwatch executes queued runs until stopped; one per hub, started by New. busy enforces "one
// run at a time" fleet-wide via TryLock, mirroring workflow/prcheck.go's preflight.
type runwatch struct {
	h    *Hub
	base context.Context // the hub's lifetime; a run in flight is a child of it (-> Hub.lifetime)
	stop chan struct{}
	done chan struct{}
	busy sync.Mutex
}

// newRunwatch starts the loop. It must not block: New runs before Serve answers the socket.
func newRunwatch(base context.Context, h *Hub) *runwatch {
	r := &runwatch{h: h, base: base, stop: make(chan struct{}), done: make(chan struct{})}
	go r.loop()
	return r
}

func (r *runwatch) loop() {
	defer close(r.done)
	t := time.NewTicker(runInterval)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
			r.tick()
		}
	}
}

// tick starts the next queued run if the fleet is idle. Execution happens off this goroutine — a
// run can take up to 15 minutes, and the ticker must keep running so a slot freeing elsewhere is
// noticed promptly rather than only once every 15 minutes.
func (r *runwatch) tick() {
	if !r.busy.TryLock() {
		return // a run is already executing fleet-wide
	}
	project, id, ok := r.h.wf.NextQueuedRun()
	if !ok {
		r.busy.Unlock()
		return
	}
	go func() {
		defer r.busy.Unlock()
		_ = r.h.wf.ExecuteRun(r.base, project, id)
	}()
}

// close stops the loop. It does not wait for a run in flight — ExecuteRun bounds itself to 15
// minutes and always finishes the run record on its own; waiting here would just delay shutdown
// by however much of that budget remained.
func (r *runwatch) close() {
	close(r.stop)
	<-r.done
}
