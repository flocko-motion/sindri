// package: hub / watchdog
// type:    logic (agent liveness observer)
// job:     own what the hub believes about the fleet — liveness, each session's fill and model,
// the pod listing, the memory headroom, each repo's docs — as the ONE place that polls for
// any of it. A board read reports the last observation; no reading flips an agent down.
// limits:  observations only; how a status word is chosen from liveness + phase stays in
// agent.AgentStatus, what headroom means in agent.Headroom, the board in state.go.
package hub

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/store"
)

const (
	// watchInterval is the REST between beats, not a period: the loop clocks on completion, so a
	// slow runtime throttles the observer instead of queueing work behind it.
	watchInterval = time.Second

	// probeEvery is how many beats apart the per-agent probes run. A sweep's two halves differ by an
	// order of magnitude: ONE listing answers for every container in a single spawn, while the
	// probes cost two spawns per agent — 24 of them at twelve agents, ~96% of the work. So the
	// cheap half runs every beat, and the dear half rides a slower one.
	probeEvery = 6

	// watchProbeParallel bounds concurrent container commands: process spawns do not parallelise
	// (24 at once ~3.2s, one ~0.2s), so a small window finishes a sweep sooner than a fan-out.
	watchProbeParallel = 4

	// downStrikes is how many consecutive failed probes declare an agent down: one contended exec
	// is not evidence. Hysteresis a per-request probe could never have, starting with no history.
	downStrikes = 3

	// probeTimeout bounds each probe: a container that can't answer reads "down", not a stalled sweep.
	// Here because the observer is the only thing in this package that probes at all.
	probeTimeout = 3 * time.Second

	// capacityInterval is how often the fleet's memory headroom is re-read. Slower than the liveness
	// cadence because it costs its own process spawn and the figure moves when an agent starts, not
	// second to second.
	capacityInterval = 10 * time.Second
)

// fill is what an agent's transcript last said: how much of its window is used, and the model
// carrying it. Sampled here because a reader taking it parses a large session file per render.
type fill struct {
	tokens, window int
	model          string
}

// liveness is what the watchdog last observed about one agent.
type liveness struct {
	fill
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
	// runtimeSince is when the runtime word last changed. A cut-off turn needs this rather than
	// stillSince: its spinner keeps animating, so the screen never stands still even though nothing
	// is happening — measured on gloin, two different digests 12s apart with a dead turn.
	runtimeSince time.Time
}

// watchdog observes agent liveness on a loop; one per hub, started by New, stopped by Close.
type watchdog struct {
	h *Hub

	mu  sync.RWMutex
	obs map[agentKey]liveness
	// runtimeErr is the last pod-listing failure, which is what an unreachable container runtime
	// looks like from here. nil once one succeeds.
	runtimeErr error
	// capacity is the last memory reading the backend gave, zero until it gives one.
	capacity container.Capacity
	// listing is the pods the last successful sweep saw — the orphan scan's question, already answered.
	listing []string
	// repos is what each repo's own files say about it, by tag: a config read and a PATH lookup.
	repos map[string]repoSample
	stop  chan struct{}
	done  chan struct{}
}

// repoSample is one repo's doc situation: the doc the board recommends, and any missing source CLI.
type repoSample struct {
	docs        RepoDocState
	specMissing bool
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
	w := &watchdog{h: h, obs: map[agentKey]liveness{}, repos: map[string]repoSample{},
		stop: make(chan struct{}), done: make(chan struct{})}
	w.seed()
	go w.loop()
	return w
}

// seed takes the cheap first reading: which pods exist, and what the repos say about themselves.
func (w *watchdog) seed() {
	w.sampleRepos()
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
	var lastCapacity time.Time
	for beat := 0; ; beat++ {
		// Beat 0 probes: the first sweep is the real first reading, off the startup path (see
		// newWatchdog), and a listing alone would leave every agent's runtime unknown until the
		// first probe beat.
		probing := beat%probeEvery == 0
		w.sweep(probing)
		// Repo files ride the probe beat: what they say moves when someone edits a repo, not per render.
		if probing {
			w.sampleRepos()
		}
		// In the loop's own goroutine rather than beside it: one observer means one process spawn
		// at a time, and a capacity sample racing a sweep is the parallelism this exists to end.
		if time.Since(lastCapacity) >= capacityInterval {
			w.sampleCapacity()
			lastCapacity = time.Now()
		}
		select {
		case <-w.stop:
			return
		case <-time.After(watchInterval):
		}
	}
}

// headroom is the fleet's memory as the board reports it: the last reading, folded into agents of
// the default size. Unknown until the backend has answered once — nobody having measured a machine
// is not the same as it having nothing free.
func (w *watchdog) headroom() api.FleetMemory {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return agent.Headroom(w.capacity)
}

// pods is the last pod listing the sweep took — what the orphan scan reads instead of listing again.
func (w *watchdog) pods() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.listing
}

// repoDocs is every registered repo's doc situation, by tag. One the sweep has not reached is absent,
// which reads as an unset doc: the next sample answers, and no reading beats one invented here.
func (w *watchdog) repoDocs() map[string]repoSample {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.repos
}

// sampleRepos re-reads what every registered repo says about itself. Held rather than read per
// render: `sindri task info` timed out past 120s while each read path took its own.
func (w *watchdog) sampleRepos() {
	projects, err := w.h.projects.Known()
	if err != nil {
		return // an unreadable registry settles nothing; the last sample stands
	}
	next := make(map[string]repoSample, len(projects))
	for _, p := range projects {
		next[p.Tag] = repoSample{docs: w.h.repoDocState(p.Path), specMissing: w.h.wf.TaskSourceToolMissing(p.Path)}
	}
	w.mu.Lock()
	w.repos = next
	w.mu.Unlock()
}

// sampleCapacity takes one reading from the backend. A failed one settles nothing, as everywhere
// else here: the previous reading stands rather than the header blinking out on a slow podman.
func (w *watchdog) sampleCapacity() {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	c, err := container.MemoryCapacity(ctx)
	if err != nil {
		return
	}
	w.mu.Lock()
	w.capacity = c
	w.mu.Unlock()
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
func (w *watchdog) sweep(withProbes bool) {
	agents, err := w.h.store.AllAgents()
	if err != nil {
		return // a store hiccup is not evidence about any agent; keep the last observations
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	existing, listErr := container.ListByLabelFresh(ctx, "sindri.project", "")
	cancel()
	// This listing IS the runtime health check, so nobody has to pay for a second one. A CLI that
	// spawned `podman info` per command was answering, in 3.8s, a question already answered here
	// every beat — and on a loaded host its own timeout misreported a slow podman as absent.
	w.mu.Lock()
	w.runtimeErr = listErr
	w.mu.Unlock()
	if listErr == nil {
		w.mu.Lock()
		w.listing = existing
		w.mu.Unlock()
	}
	exists := make(map[string]bool, len(existing))
	for _, p := range existing {
		exists[p] = true
	}

	sem := make(chan struct{}, watchProbeParallel)
	var wg sync.WaitGroup
	for _, a := range agents {
		gone := listErr == nil && !exists[w.h.container(a.Project, a.Name)]
		if gone {
			w.record(a, false, 0, agent.Observation{})
		}
		// A pod that EXISTS says nothing yet about the session inside it, which only the probe
		// answers — so on a listing-only beat its last observation stands untouched. Death is still
		// caught at full speed: absence above is conclusive on every beat.
		if !withProbes {
			continue
		}
		wg.Add(1)
		go func(a store.Agent, gone bool) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if !gone {
				w.probe(a)
			}
			// After the liveness reading, or a fill would create the entry and an unobserved agent
			// would read down rather than unknown. Off the host's disk, so a stopped pod still answers.
			if t, win, m, ok := w.h.agents.SampleContext(a.Project, a.Name); ok {
				w.recordFill(a, fill{tokens: t, window: win, model: m})
			}
		}(a, gone)
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
	// The fill rides along: dropping it here would blank the board's context column every sweep.
	next := liveness{fill: prev.fill, up: up, clients: clients, runtime: obs.Runtime, digest: obs.Digest, seen: time.Now()}
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
	// Set after the word is final, so it measures the state as reported rather than as read.
	if next.runtimeSince = prev.runtimeSince; next.runtime != prev.runtime || next.runtimeSince.IsZero() {
		next.runtimeSince = next.seen
	}
	w.obs[key] = next
}

// forgetFill drops one agent's fill: a window of 0 is the unknown every reader already handles, and
// the next sweep measures again. For the moment a clear or a compact makes the last reading false.
func (w *watchdog) forgetFill(project, name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := agentKey{project, name}
	if l, seen := w.obs[key]; seen {
		l.fill = fill{}
		w.obs[key] = l
	}
}

// recordFill folds in one transcript reading. Apart from record because it has no hysteresis to
// share: a sample that read nothing is not evidence of an empty context, so the caller does not call.
func (w *watchdog) recordFill(a store.Agent, f fill) {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := agentKey{a.Project, a.Name}
	l, seen := w.obs[key]
	if !seen {
		return // nothing has observed this agent yet, and a fill alone is not an observation of it
	}
	l.fill = f
	w.obs[key] = l
}
