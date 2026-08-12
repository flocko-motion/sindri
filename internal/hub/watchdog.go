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
	"fmt"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
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
	runtime string // Claude's live runtime: working|blocked|idle|signed-out|""
	digest  string // the pane's content hash, so stillness is measurable
	strikes int    // consecutive failed probes; up is held until downStrikes
	seen    time.Time
	// stillSince is when the pane last changed. A dwell rather than a flag: a tool call holds the
	// screen still for its duration (measured: 12s+ on a working agent), a stall holds it still
	// indefinitely, and only the length tells them apart.
	stillSince time.Time
}

// watchdog observes agent liveness on a loop; one per hub, started by New, stopped by Close.
type watchdog struct {
	h *Hub

	mu  sync.RWMutex
	obs map[agentKey]liveness
	// runtimeErr is the last pod-listing failure, which is what an unreachable container runtime
	// looks like from here. nil once one succeeds.
	runtimeErr error
	stop       chan struct{}
	done       chan struct{}
}

// runtimeHint reports why the container runtime looks unreachable, "" when it answers. Read off the
// sweep the hub already runs, so asking costs nothing.
func (w *watchdog) runtimeHint() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.runtimeErr == nil {
		return ""
	}
	// Worded from the failure in hand rather than by asking the backend again: Healthy() spawns its
	// own probe, which is the cost this exists to avoid.
	return fmt.Sprintf("%s isn't answering (%v) — agents can't run until it is", container.Name(), w.runtimeErr)
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
		w.record(a, exists[w.h.container(a.Project, a.Name)], 0, agent.Observation{})
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
	// This listing IS the runtime health check, so nobody has to pay for a second one. A CLI that
	// spawned `podman info` per command was answering, in 3.8s, a question already answered here
	// every 2 seconds — and on a loaded host its own timeout misreported a slow podman as absent.
	w.mu.Lock()
	w.runtimeErr = listErr
	w.mu.Unlock()
	exists := make(map[string]bool, len(existing))
	for _, p := range existing {
		exists[p] = true
	}

	sem := make(chan struct{}, watchProbeParallel)
	var wg sync.WaitGroup
	for _, a := range agents {
		if listErr == nil && !exists[w.h.container(a.Project, a.Name)] {
			w.record(a, false, 0, agent.Observation{})
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
		w.record(a, false, 0, agent.Observation{})
		return
	}
	obs := w.h.agents.Observe(ctx, a.Project, a.Name)
	w.record(a, true, len(cs), obs)
}

// record folds one observation in: a success clears strikes, a failure holds the previous state and
// its counts until downStrikes. No single reading settles anything, whatever its source — a missing
// pod and a failed probe are both one observation, and a listing can be a moment out of date.
func (w *watchdog) record(a store.Agent, up bool, clients int, obs agent.Observation) {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := agentKey{a.Project, a.Name}
	prev := w.obs[key]
	next := liveness{up: up, clients: clients, runtime: obs.Runtime, digest: obs.Digest, seen: time.Now()}
	switch {
	case up:
		next.strikes = 0
	default:
		next.strikes = prev.strikes + 1
		if next.strikes < downStrikes && prev.up {
			// Not yet convinced: keep what the last good probe saw.
			next.up, next.clients, next.runtime, next.digest = true, prev.clients, prev.runtime, prev.digest
		}
	}
	// The screen changing is the one direct evidence of an agent doing something, and the classifier
	// is a reading of words that may be minutes old. A failed capture has no digest and settles
	// nothing — it carries the dwell rather than restarting it, so a lost probe cannot hide a stall.
	switch {
	case next.digest == "":
		next.stillSince = prev.stillSince
	case next.digest != prev.digest:
		next.stillSince = next.seen
	default:
		if next.stillSince = prev.stillSince; next.stillSince.IsZero() {
			next.stillSince = next.seen
		}
	}
	// Activity decides working-vs-idle wherever the text did not settle it. "idle" is what the
	// classifier says for any screen it doesn't recognise, so on its own it made a busy agent with
	// an unfamiliar pane look stopped.
	if next.runtime == "idle" && next.digest != "" && next.digest != prev.digest && prev.digest != "" {
		next.runtime = "working"
	}
	w.obs[key] = next
}
