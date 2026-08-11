// package: hub / watchdog
// type:    logic (agent liveness observer)
// job:     own what the hub believes about every agent's liveness — one loop probing on a
// fixed cadence, so a board read reports the last observation instead of taking
// one, and no single reading — a lost probe, or a listing taken a moment ago —
// flips an agent to "down".
// limits:  liveness and dial-in counts only; how a status word is chosen from liveness +
// phase stays in agent.AgentStatus, and the board assembly in state.go.
package hub

import (
	"context"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

const (
	// watchInterval is the observation cadence — under the TUI's 3s poll, so no reading is wasted.
	watchInterval = 2 * time.Second

	// watchProbeParallel bounds concurrent container commands: process spawns do not parallelise
	// (24 at once ~3.2s, one ~0.2s), so a small window finishes a sweep sooner than a fan-out.
	watchProbeParallel = 4

	// downStrikes is how many consecutive failed probes declare an agent down: one contended exec
	// is not evidence. Hysteresis a per-request probe could never have, starting with no history.
	downStrikes = 3
)

// liveness is what the watchdog last observed about one agent.
type liveness struct {
	up      bool
	clients int
	runtime string // Claude's live runtime: working|blocked|idle|""
	strikes int    // consecutive failed probes; up is held until downStrikes
	seen    time.Time
	// idleSince is when the runtime first read "idle" and has read nothing else since; zero
	// whenever it isn't idle. A dwell rather than a flag: thinking pauses read idle for a
	// moment, a stall reads idle for minutes, and only the length tells them apart.
	idleSince time.Time
}

// watchdog observes agent liveness on a loop; one per hub, started by New, stopped by Close.
type watchdog struct {
	h *Hub

	mu   sync.RWMutex
	obs  map[agentKey]liveness
	stop chan struct{}
	done chan struct{}
}

// newWatchdog builds and starts the observer. It must not block — New runs before Serve answers
// the socket, and a full sweep (14 agents × 2 commands, 4-wide) delayed startup past the health
// check — so the first pass only lists, provisionally, and the first sweep refines it.
func newWatchdog(h *Hub) *watchdog {
	w := &watchdog{h: h, obs: map[agentKey]liveness{}, stop: make(chan struct{}), done: make(chan struct{})}
	w.seed()
	go w.loop()
	return w
}

// seed takes the cheap first reading: which pods exist, and nothing else.
func (w *watchdog) seed() {
	agents, err := w.h.store.AllAgents()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	existing, listErr := container.ListByLabelFresh(ctx, "sindri.project", "")
	cancel()
	if listErr != nil {
		return // nothing observed; the first sweep will fill it in
	}
	exists := make(map[string]bool, len(existing))
	for _, p := range existing {
		exists[p] = true
	}
	for _, a := range agents {
		// Both provisional: the sweep refines them. Absence is a reading like any other, and one
		// reading never settles anything on its own.
		w.record(a, exists[w.h.container(a.Project, a.Name)], 0, "")
	}
}

// loop observes the fleet until stopped.
func (w *watchdog) loop() {
	defer close(w.done)
	w.sweep() // the real first reading, off the startup path (see newWatchdog)
	t := time.NewTicker(watchInterval)
	defer t.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-t.C:
			w.sweep()
		}
	}
}

// close stops the loop and waits for the sweep in flight.
func (w *watchdog) close() {
	close(w.stop)
	<-w.done
}

// get is the last observation for an agent, and whether there is one at all.
func (w *watchdog) get(project, name string) (liveness, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	l, ok := w.obs[agentKey{project, name}]
	return l, ok
}

// sweep reads the fleet: one listing of which pods exist — cheap, it answers for every container at
// once — then a tmux probe per agent that has one. The listing is taken fresh: this is the caller
// whose question is about now, and a memoized answer predating a launch reports the new pod absent.
func (w *watchdog) sweep() {
	agents, err := w.h.store.AllAgents()
	if err != nil {
		return // a store hiccup is not evidence about any agent; keep the last observations
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	existing, listErr := container.ListByLabelFresh(ctx, "sindri.project", "")
	cancel()
	exists := make(map[string]bool, len(existing))
	for _, p := range existing {
		exists[p] = true
	}

	sem := make(chan struct{}, watchProbeParallel)
	var wg sync.WaitGroup
	for _, a := range agents {
		if listErr == nil && !exists[w.h.container(a.Project, a.Name)] {
			w.record(a, false, 0, "")
			continue
		}
		wg.Add(1)
		go func(a store.Agent) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			w.probe(a)
		}(a)
	}
	wg.Wait()
}

// probe reads one agent's tmux session and, when up, Claude's state; a failure is a strike only.
func (w *watchdog) probe(a store.Agent) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cs, ok := w.h.agents.ClientsCtx(ctx, a.Project, a.Name)
	if !ok {
		w.record(a, false, 0, "")
		return
	}
	rt := w.h.agents.RuntimeState(ctx, a.Project, a.Name)
	w.record(a, true, len(cs), rt)
}

// record folds one observation in: a success clears strikes, a failure holds the previous state and
// its counts until downStrikes. No single reading settles anything, whatever its source — a missing
// pod and a failed probe are both one observation, and a listing can be a moment out of date.
func (w *watchdog) record(a store.Agent, up bool, clients int, runtime string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := agentKey{a.Project, a.Name}
	prev := w.obs[key]
	next := liveness{up: up, clients: clients, runtime: runtime, seen: time.Now()}
	switch {
	case up:
		next.strikes = 0
	default:
		next.strikes = prev.strikes + 1
		if next.strikes < downStrikes && prev.up {
			// Not yet convinced: keep what the last good probe saw.
			next.up, next.clients, next.runtime = true, prev.clients, prev.runtime
		}
	}
	// After the switch, so a held-over runtime carries its dwell too — a lost probe mid-stall
	// must not restart the clock and hide the stall for another full dwell.
	if next.runtime == "idle" {
		if next.idleSince = prev.idleSince; next.idleSince.IsZero() {
			next.idleSince = next.seen
		}
	}
	w.obs[key] = next
}
