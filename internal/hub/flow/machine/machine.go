// package: hub/flow/machine / machine
// type:    logic (the flow engine: stored state, per-state observers, one action at a time)
// job:     run a declared flow — hold each subject's stored state, run that state's action, watch
// its conditions on their own cadence and on their topics, and move the subject when something
// declared happens.
// limits:  the engine. What a state MEANS, what an action does and what a condition reads belong to
// the flow plugged into it; this package names none of them.
package machine

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Machine runs one declared flow over many subjects.
type Machine[W any] interface {
	// Wake asks for one subject to be looked at now, because topic happened. A HINT: a dropped one
	// costs at most that state's Every, never correctness.
	Wake(subject string, topic Topic)
	// Look runs ONE pass over a subject now, rather than at its own next beat: gather, check every
	// condition, and start the state's action if it has one. For a caller that wants the answer
	// before it returns — and for a test, which cannot wait on a beat.
	Look(subject string)
	// State is where a subject stands, and the declaration of that state.
	State(subject string) (State[W], error)
	// Would is where a subject would land if it were looked at now — the conditions followed, and
	// nothing written. For a caller that must agree with the machine WITHOUT waiting for its next
	// pass: the alternative is a second copy of the rules, and a second copy is what drifts.
	Would(subject string) (State[W], error)
	// Weigh is where a subject stands and which of that state's events hold now, in declaration
	// order — the first true one is the edge a pass would take. Nothing written.
	Weigh(subject string) (State[W], []bool, error)
	// States is the declared flow, in registration order — the map, printable.
	States() []State[W]
	Close() error
}

// Config is what a domain supplies to run its flow.
type Config[W any] struct {
	// States is the flow. Every name unique, every Action registered, every state with an action
	// carrying at least one condition exit (-> check).
	States []State[W]
	// Start is the state a subject with none stored is put into.
	Start string
	// Gather reads the world for one subject, ONCE per pass, before any condition looks at it.
	Gather func(subject string) (W, error)
	// Stored reads a subject's current state name and WHEN it was entered — "" when it has none yet.
	// The entry time is what tells a state just reached from one a dead hub left behind.
	Stored func(subject string) (name string, since time.Time, err error)
	// Move writes a subject's new state, with the words the transition carried.
	Move func(subject, from, to, why string) error
	// Do is the implementation of each declared action, by name.
	Do map[string]Doer[W]
	// Subjects lists everything the machine watches.
	Subjects func() []string
	// Superseded maps a state this flow NO LONGER declares to the declared one that replaces it. A
	// subject's stored state outlives the map that wrote it, so every removal needs a destination
	// here or the rows naming it resolve to nothing and the subject is stranded.
	Superseded map[string]string
	// Default is the poll cadence for a state that declares no Every of its own.
	Default time.Duration
	// Tick is how often the engine looks for subjects whose poll has come due. ZERO runs no loop at
	// all: the machine still answers where a subject stands and runs a pass on demand (-> Look), but
	// nothing happens on its own. That is what a machine outside the hub — a test, a one-off — should be.
	Tick time.Duration
	// Record receives every step of every pass.
	Record Recorder
}

// running is the one action in flight for a subject. done closes when it returns, so a caller that
// is holding the line can wait for the action the loop started rather than reporting over the top
// of it.
type running struct {
	action string
	pass   string
	cancel context.CancelFunc
	done   chan struct{}
	// awaits carries the action's own declaration (-> Action.Awaits), so a caller holding the line
	// knows not to wait behind this one without looking up where the subject stands again.
	awaits bool
}

type machine[W any] struct {
	cfg      Config[W]
	states   map[string]State[W]
	lifetime context.Context
	stop     context.CancelFunc

	mu    sync.Mutex
	due   map[string]time.Time
	act   map[string]*running
	woken map[string]bool
	// gates make a PASS atomic per subject. Every look used to happen on the loop's one goroutine,
	// which is no longer true: a caller asks for one on its own (-> Look). Two passes reading one
	// subject at once each decided from the state before the other moved it — which is how an agent
	// came to be handed work by one pass and freed by the next for holding none.
	gates map[string]*sync.Mutex
	// ran is every (subject, action) this PROCESS has started. An acting state with no entry here
	// was entered by a hub that is gone, which is what Orphaned reports.
	ran map[string]bool

	// started is when this machine came up, TRUNCATED to the second: entry times are recorded at that
	// granularity, so a state entered milliseconds after startup would otherwise parse as earlier
	// than it and read as orphaned. The cost is that a state entered in the same second as a restart
	// is not spotted as orphaned until the one after; the gain is that a state just entered is never
	// mistaken for one a dead hub left behind.
	started time.Time

	poke      chan struct{}
	loop      sync.WaitGroup
	actions   sync.WaitGroup
	closeOnce sync.Once
	passes    counter
}

// New checks a flow and starts running it until lifetime ends or Close returns.
func New[W any](lifetime context.Context, cfg Config[W]) (Machine[W], error) {
	m := &machine[W]{
		cfg: cfg, states: make(map[string]State[W], len(cfg.States)),
		due: map[string]time.Time{}, act: map[string]*running{}, woken: map[string]bool{},
		gates: map[string]*sync.Mutex{},
		ran:   map[string]bool{},
		poke:  make(chan struct{}, 1), started: time.Now().Truncate(time.Second),
	}
	for _, s := range cfg.States {
		if _, dup := m.states[s.Name]; dup {
			return nil, fmt.Errorf("machine: two states declared as %q", s.Name)
		}
		m.states[s.Name] = s
	}
	if err := m.check(); err != nil {
		return nil, err
	}
	m.lifetime, m.stop = context.WithCancel(lifetime)
	if cfg.Tick > 0 {
		m.loop.Add(1)
		go m.loopUntilClosed()
	}
	return m, nil
}

// check holds a flow to the rules that make it readable and recoverable.
func (m *machine[W]) check() error {
	if _, ok := m.states[m.cfg.Start]; !ok {
		return fmt.Errorf("machine: the start state %q is not declared", m.cfg.Start)
	}
	for _, s := range m.states {
		if s.Title == "" || s.About == "" {
			return fmt.Errorf("machine: state %q must declare a Title and an About", s.Name)
		}
		if err := m.checkExits(s); err != nil {
			return err
		}
	}
	for gone, to := range m.cfg.Superseded {
		if _, still := m.states[gone]; still {
			return fmt.Errorf("machine: %q is declared AND listed as superseded — a live state has no replacement", gone)
		}
		if _, ok := m.states[to]; !ok {
			return fmt.Errorf("machine: %q is superseded by %q, which is not declared", gone, to)
		}
	}
	return nil
}

// checkExits is the rule the unified event list makes necessary: an action's outcomes are the only
// EDGE-triggered events, so a state whose exits are all outcomes is unreachable again the moment
// that action dies. Every state that acts must also declare a condition somebody can observe.
func (m *machine[W]) checkExits(s State[W]) error {
	conditions := 0
	for _, t := range s.Events {
		if _, ok := m.states[t.To]; !ok && t.To != Stay {
			return fmt.Errorf("machine: state %q leads to undeclared %q", s.Name, t.To)
		}
		if _, isOutcome := t.On.(Outcome); !isOutcome {
			conditions++ // a Condition or Orphaned: something an observer can reach
		}
	}
	if s.Action == nil {
		return nil
	}
	if _, ok := m.cfg.Do[s.Action.Name]; !ok {
		return fmt.Errorf("machine: state %q runs action %q, which has no implementation", s.Name, s.Action.Name)
	}
	if conditions == 0 {
		return fmt.Errorf("machine: state %q runs an action but declares no condition exit — "+
			"an action that dies would leave the subject there for ever", s.Name)
	}
	for _, want := range s.Action.Outcomes {
		if !handles(s, want) {
			return fmt.Errorf("machine: state %q runs %q but does not say where %q leads",
				s.Name, s.Action.Name, want.Name)
		}
	}
	return nil
}

// handles reports whether a state says where one outcome leads.
func handles[W any](s State[W], want Outcome) bool {
	for _, t := range s.Events {
		if o, ok := t.On.(Outcome); ok && o.Name == want.Name {
			return true
		}
	}
	return false
}

// States is the declared flow in registration order.
func (m *machine[W]) States() []State[W] {
	return append(make([]State[W], 0, len(m.cfg.States)), m.cfg.States...)
}

// State is where a subject stands, and what that state declares.
func (m *machine[W]) State(subject string) (State[W], error) {
	s, _, err := m.standing(subject)
	return s, err
}

// standing is where a subject stands and when it got there.
func (m *machine[W]) standing(subject string) (State[W], time.Time, error) {
	name, since, err := m.cfg.Stored(subject)
	if err != nil {
		return State[W]{}, since, err
	}
	if name == "" {
		name = m.cfg.Start
	}
	if s, ok := m.states[name]; ok {
		return s, since, nil
	}
	// A name the map no longer has is STALE DATA, not a fault in the subject: the flow was edited
	// under a row an older one wrote. Stranding it fails every call that subject makes for as long
	// as the row stands, which is how deleting one state took a worker off the board entirely.
	to, listed := m.cfg.Superseded[name]
	why := "the state it stood in was replaced by " + to
	if !listed {
		// Nobody said where this one went, so the start is the only safe answer — and saying so is
		// the point: the omission is a missing Superseded entry, not something to swallow.
		to, why = m.cfg.Start, "the state it stood in is no longer declared, and nothing says what replaced it"
	}
	m.record(Entry{Subject: subject, State: name, Step: StepMoved, Detail: name + " -> " + to + ": " + why})
	if err := m.cfg.Move(subject, name, to, why); err != nil {
		return State[W]{}, since, fmt.Errorf("machine: %s stands in %q, which is not declared, and it could not be moved to %q: %w", subject, name, to, err)
	}
	return m.states[to], time.Now(), nil
}

// gate is one subject's pass lock, made on first use.
func (m *machine[W]) gate(subject string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.gates[subject]
	if !ok {
		g = &sync.Mutex{}
		m.gates[subject] = g
	}
	return g
}

// Close stops the loop and every action in flight.
func (m *machine[W]) Close() error {
	m.closeOnce.Do(func() {
		m.stop()
		m.loop.Wait()
		m.actions.Wait()
	})
	return nil
}
