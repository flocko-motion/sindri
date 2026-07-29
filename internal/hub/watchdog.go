// package: hub / watchdog
// type:    logic (agent liveness observer)
// job:     own what the hub believes about every agent's liveness — one loop probing on a
//          fixed cadence, so a board read reports the last observation instead of taking
//          one, and a single lost probe never flips an agent to "down".
// limits:  liveness and dial-in counts only; how a status word is chosen from liveness +
//          phase stays in agent.AgentStatus, and the board assembly in state.go.
package hub

import (
	"context"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

const (
	// watchInterval is how often the fleet is observed. Under the TUI's 3s poll, so the board
	// is never showing an observation the next poll would have replaced anyway.
	watchInterval = 2 * time.Second

	// watchProbeParallel bounds concurrent container commands. Each is a process spawn, and
	// spawns do not parallelise — 24 at once measure ~3.2s where one is ~0.2s — so a small
	// window finishes a sweep sooner than a full fan-out does.
	watchProbeParallel = 4

	// downStrikes is how many consecutive failed probes declare an agent down. A probe that
	// loses a race is not evidence: the previous ones said the agent was up, and one
	// contended exec cannot outweigh them. This is the hysteresis a per-request probe could
	// never have, because each request started with no history.
	downStrikes = 3
)

// liveness is what the watchdog last observed about one agent.
type liveness struct {
	up      bool
	clients int
	runtime string // Claude's live runtime: working|blocked|idle|""
	strikes int    // consecutive failed probes; up is held until downStrikes
	seen    time.Time
}

// watchdog observes agent liveness on a loop. One instance per hub, started by New and
// stopped by Close.
type watchdog struct {
	h *Hub

	mu   sync.RWMutex
	obs  map[agentKey]liveness
	stop chan struct{}
	done chan struct{}
}

// newWatchdog builds the observer and starts it. It must not block: New runs before Serve
// answers the socket, so a full sweep here delayed the hub's own startup past the health check
// — 14 agents at two container commands each, behind a 4-wide gate, is seconds of podman.
//
// A listing-only first pass is the compromise. One command (~50ms) is cheap enough to wait for
// and settles most of the fleet correctly: a pod that is absent is conclusively down. Pods that
// exist are taken as up provisionally, which the first real sweep confirms or corrects within a
// tick — better than reporting a running fleet as down for the first two seconds.
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
	existing, listErr := container.ListByLabelCached(ctx, "sindri.project", "")
	cancel()
	if listErr != nil {
		return // nothing observed; the first sweep will fill it in
	}
	exists := make(map[string]bool, len(existing))
	for _, p := range existing {
		exists[p] = true
	}
	for _, a := range agents {
		if up := exists[w.h.container(a.Project, a.Name)]; up {
			w.record(a, true, 0, "", false) // provisional: the sweep refines it
		} else {
			w.record(a, false, 0, "", true) // absent is conclusive
		}
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

// sweep takes one reading of the whole fleet: one listing for "which pods exist", then a tmux
// probe per agent that has one.
//
// The listing is what makes this cheap — it answers for every container at once, so the
// per-agent cost applies only to agents whose pod is actually there. An agent with no pod is
// down immediately, with no probe and no strikes: absence is conclusive.
func (w *watchdog) sweep() {
	agents, err := w.h.store.AllAgents()
	if err != nil {
		return // a store hiccup is not evidence about any agent; keep the last observations
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	existing, listErr := container.ListByLabelCached(ctx, "sindri.project", "")
	cancel()
	exists := make(map[string]bool, len(existing))
	for _, p := range existing {
		exists[p] = true
	}

	sem := make(chan struct{}, watchProbeParallel)
	var wg sync.WaitGroup
	for _, a := range agents {
		if listErr == nil && !exists[w.h.container(a.Project, a.Name)] {
			w.record(a, false, 0, "", true)
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

// probe observes one agent: its tmux session (liveness plus the dial-in count in one command)
// and, when up, what Claude is doing. A failure records a strike rather than a verdict.
func (w *watchdog) probe(a store.Agent) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cs, ok := w.h.agents.ClientsCtx(ctx, a.Project, a.Name)
	if !ok {
		w.record(a, false, 0, "", false)
		return
	}
	rt := w.h.agents.RuntimeState(ctx, a.Project, a.Name)
	w.record(a, true, len(cs), rt, false)
}

// record folds one observation into the agent's liveness, applying the strike rule.
//
// conclusive marks a verdict that needs no corroboration — the pod is absent from the listing,
// so there is nothing to be racing. Everything else builds or clears strikes: a success clears
// them at once, while a failure only reports down once downStrikes have accumulated. Until
// then the agent keeps its previous state, and keeps its dial-in count and runtime with it,
// so a contended probe does not blank the row it failed to read.
func (w *watchdog) record(a store.Agent, up bool, clients int, runtime string, conclusive bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := agentKey{a.Project, a.Name}
	prev := w.obs[key]
	next := liveness{up: up, clients: clients, runtime: runtime, seen: time.Now()}
	switch {
	case up:
		next.strikes = 0
	case conclusive:
		next.strikes = downStrikes
	default:
		next.strikes = prev.strikes + 1
		if next.strikes < downStrikes && prev.up {
			// Not yet convinced: keep what the last good probe saw.
			next.up, next.clients, next.runtime = true, prev.clients, prev.runtime
		}
	}
	w.obs[key] = next
}
