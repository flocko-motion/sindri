// package: hub/flow/machine / loop
// type:    logic (running the flow: cadence, observers, one action per subject)
// job:     look at a subject when its state's poll comes due or a topic wakes it, run that state's
// action once, and move it the moment something the state declared happens.
// limits:  the mechanics. The declarations are state.go's and the registry is machine.go's.
package machine

import (
	"context"
	"fmt"
	"time"
)

// run is the machine's single owner: every look happens on this goroutine, so a subject's state and
// the action running for it are read in one well-defined turn.
func (m *machine[W]) loopUntilClosed() {
	defer m.loop.Done()
	t := time.NewTicker(m.cfg.Tick)
	defer t.Stop()
	for {
		select {
		case <-m.lifetime.Done():
			return
		case <-m.poke:
			m.sweep(true)
		case <-t.C:
			m.sweep(false)
		}
	}
}

// Wake marks a subject for a look now, but ONLY if where it stands watches for that topic — the
// declared subscriptions are what make a topic cheap enough to publish freely. It never blocks: a
// second wake before the first is served collapses into it.
func (m *machine[W]) Wake(subject string, topic Topic) {
	if m.cfg.Tick <= 0 {
		return // no loop is running, so nothing would drain the mark
	}
	if !m.listens(subject, topic) {
		return
	}
	m.mu.Lock()
	m.woken[subject] = true
	delete(m.due, subject) // due now, whatever its cadence said
	m.mu.Unlock()
	select {
	case m.poke <- struct{}{}:
	default:
	}
}

// listens reports whether where a subject stands watches for this topic. An empty topic is a general
// prompt — something moved, nobody knows what — and reaches every subject.
func (m *machine[W]) listens(subject string, topic Topic) bool {
	if topic == "" {
		return true
	}
	s, err := m.State(subject)
	if err != nil {
		return true // it stands somewhere undeclared; a pass is how that gets reported
	}
	for _, t := range s.Events {
		c, ok := t.On.(Condition[W])
		if !ok {
			continue
		}
		for _, w := range c.Wake {
			if w == topic {
				return true
			}
		}
	}
	return false
}

// sweep looks at every subject that is due, or every woken one when a wake prompted this.
func (m *machine[W]) sweep(woken bool) {
	for _, subject := range m.pick(woken) {
		if m.lifetime.Err() != nil {
			return
		}
		m.look(subject, false)
	}
}

// pick is the subjects to look at now: the woken ones on a wake, the due ones on a tick.
func (m *machine[W]) pick(woken bool) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if woken {
		out := make([]string, 0, len(m.woken))
		for s := range m.woken {
			out = append(out, s)
		}
		clear(m.woken)
		return out
	}
	if m.cfg.Subjects == nil {
		return nil
	}
	now := time.Now()
	var out []string
	for _, s := range m.cfg.Subjects() {
		if d, seen := m.due[s]; !seen || !now.Before(d) {
			out = append(out, s)
		}
	}
	return out
}

// settleCap bounds one Look. A pass that moves the subject earns another, and a real chain runs to
// several — a conflict resolved, a milestone caught up with, a feature re-entered, a subtask taken —
// so this is generous. A flow that never settles is a bug, and stopping beats spinning.
const settleCap = 12

// Look runs passes on the caller's goroutine until the subject settles, so an asker gets the answer
// AFTER the world moved. The loop's passes go through the same function.
func (m *machine[W]) Look(subject string) {
	for range settleCap {
		before, _, err := m.cfg.Stored(subject)
		if err != nil {
			return
		}
		m.look(subject, true)
		if after, _, err := m.cfg.Stored(subject); err != nil || after == before {
			return
		}
	}
}

// Would follows this subject's conditions from where it stands, writing nothing, and reports where
// they run out. The world is gathered ONCE, as a real pass gathers it.
func (m *machine[W]) Would(subject string) (State[W], error) {
	s, since, err := m.standing(subject)
	if err != nil {
		return State[W]{}, err
	}
	w, gerr := m.cfg.Gather(subject)
	if gerr != nil {
		return s, gerr
	}
	for range settleCap {
		t, ok := m.observed(subject, s, w, since)
		if !ok {
			// An acting state is a place a subject passes THROUGH: stopping at one answers with a
			// moment rather than a destination, so follow where its action expects to land.
			if t, ok = m.expected(s); !ok {
				return s, nil
			}
		}
		next, declared := m.states[t.To]
		if t.To == Stay || !declared {
			return s, nil
		}
		s = next
	}
	return s, nil
}

// expected is where this state's action means to leave, by its first declared outcome. A prediction
// that follows the map's own declaration is not a second copy of the rules.
func (m *machine[W]) expected(s State[W]) (Transition[W], bool) {
	if s.Action == nil || len(s.Action.Outcomes) == 0 {
		return Transition[W]{}, false
	}
	want := s.Action.Outcomes[0]
	for _, t := range s.Events {
		if o, ok := t.On.(Outcome); ok && o.Name == want.Name {
			return t, true
		}
	}
	return Transition[W]{}, false
}

// look is one pass over one subject: read where it stands, start its action if it has one and none
// is running, then ask every condition the state declared.
func (m *machine[W]) look(subject string, wait bool) {
	// One pass at a time per subject. A caller holding the line WAITS its turn; the loop SKIPS a
	// subject somebody is already deciding, and comes round to it again — a dropped look costs a
	// beat, which is the same bargain every topic makes.
	g := m.gate(subject)
	if wait {
		g.Lock()
	} else if !g.TryLock() {
		return
	}
	defer g.Unlock()
	pass := m.passes.next()
	s, since, err := m.standing(subject)
	if err != nil {
		m.record(Entry{Pass: pass, Subject: subject, Step: StepFailed, Detail: "state", Err: err})
		return
	}
	m.reschedule(subject, s)
	w, err := m.cfg.Gather(subject)
	if err != nil {
		m.record(Entry{Pass: pass, Subject: subject, State: s.Name, Step: StepFailed, Detail: "gather", Err: err})
		return
	}
	if t, ok := m.observed(subject, s, w, since); ok {
		m.move(pass, subject, s, t)
		return
	}
	m.begin(pass, subject, s, w, wait)
}

// reschedule sets when this subject is next due. The cadence is DERIVED — the freshest answer any of
// this state's conditions asks for — so no map carries a number somebody chose by feel.
func (m *machine[W]) reschedule(subject string, s State[W]) {
	m.mu.Lock()
	m.due[subject] = time.Now().Add(cadence(s, m.cfg.Default))
	m.mu.Unlock()
}

// cadence is the shortest staleness any of a state's exits will tolerate, or fallback when it
// tolerates any — a state whose every exit arrives by topic or outcome need not be polled at all.
func cadence[W any](s State[W], fallback time.Duration) time.Duration {
	every := time.Duration(0)
	for _, t := range s.Events {
		var within time.Duration
		switch c := t.On.(type) {
		case Condition[W]:
			within = c.Within
		case Orphaned:
			within = OrphanCheck
		}
		if within > 0 && (every == 0 || within < every) {
			every = within
		}
	}
	if every > 0 {
		return every
	}
	if fallback > 0 {
		return fallback
	}
	return time.Minute
}

// observed is the first event of this state the world agrees with, in declaration order. Outcomes
// are skipped: the engine owns those, and they arrive with the action rather than from the world.
func (m *machine[W]) observed(subject string, s State[W], w W, since time.Time) (Transition[W], bool) {
	for _, t := range s.Events {
		switch c := t.On.(type) {
		case Condition[W]:
			if c.Holds != nil && c.Holds(w) {
				return t, true
			}
		case Orphaned:
			if m.orphaned(subject, s, since) {
				return t, true
			}
		}
	}
	return Transition[W]{}, false
}

// orphaned reports an acting state entered BEFORE this machine started with nothing running — a hub
// that died mid-action. Entry time is what separates it from a state just reached.
func (m *machine[W]) orphaned(subject string, s State[W], since time.Time) bool {
	if s.Action == nil || !since.Before(m.started) {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.act[subject] == nil && !m.ran[subject+"\x00"+s.Action.Name]
}

// begin starts the state's action, if it has one and nothing is running for this subject. The action
// runs under its own cancellable ctx, and its outcome moves the subject when it returns.
func (m *machine[W]) begin(pass, subject string, s State[W], w W, wait bool) {
	if s.Action == nil {
		return // the hub does nothing here; the subject is working, or waiting on somebody else
	}
	m.mu.Lock()
	if in := m.act[subject]; in != nil {
		m.mu.Unlock()
		if wait && !in.awaits {
			// The loop got there first. A caller holding the line WAITS rather than reporting over the
			// top of it: answering from a world mid-move is Look going missing. An action that waits on
			// the subject is the exception, and the reason Awaits exists: the answer it is waiting for
			// can only arrive from the subject, which is who is holding this line.
			select {
			case <-in.done:
			case <-m.lifetime.Done():
			}
		}
		return
	}
	ctx, cancel := context.WithCancel(m.lifetime)
	settled := make(chan struct{})
	m.act[subject] = &running{action: s.Action.Name, pass: pass, cancel: cancel, done: settled,
		awaits: s.Action.Awaits}
	m.ran[subject+"\x00"+s.Action.Name] = true
	m.mu.Unlock()
	m.record(Entry{Pass: pass, Subject: subject, State: s.Name, Step: StepStarted, Detail: s.Action.Name})
	m.actions.Add(1)
	run := func() {
		defer m.actions.Done()
		defer cancel()
		defer close(settled)
		out, err := m.cfg.Do[s.Action.Name](ctx, w)
		m.mu.Lock()
		if in := m.act[subject]; in != nil && in.pass == pass {
			delete(m.act, subject)
		}
		delete(m.due, subject) // its outcome is news: look again at once
		m.mu.Unlock()
		if err != nil {
			m.record(Entry{Pass: pass, Subject: subject, State: s.Name, Step: StepFailed,
				Detail: s.Action.Name, Err: err})
			return
		}
		m.record(Entry{Pass: pass, Subject: subject, State: s.Name, Step: StepOutcome, Detail: out.Name})
		m.landed(pass, subject, s, out)
	}
	if wait && !s.Action.Awaits {
		run() // the caller is holding the line for the answer
		return
	}
	go run()
}

// landed moves the subject where its state said this outcome leads.
func (m *machine[W]) landed(pass, subject string, s State[W], out Outcome) {
	for _, t := range s.Events {
		if o, ok := t.On.(Outcome); ok && o.Name == out.Name {
			m.move(pass, subject, s, t)
			return
		}
	}
	m.record(Entry{Pass: pass, Subject: subject, State: s.Name, Step: StepFailed,
		Detail: "outcome " + out.Name, Err: fmt.Errorf("the state does not say where %q leads", out.Name)})
}

// move writes the subject's new state and says what moved it. An action still running for the state
// being left is cancelled first: whatever it was doing is no longer what the subject is here for.
func (m *machine[W]) move(pass, subject string, from State[W], t Transition[W]) {
	if t.To == Stay {
		// Something worth acting on that moves nobody. Recorded, so the prod that fired is on the
		// agent's account, and left where it was.
		m.record(Entry{Pass: pass, Subject: subject, State: from.Name, Step: StepMoved,
			Detail: t.On.EventName() + " -> (stays): " + t.Why})
		return
	}
	m.cancel(subject, pass, "leaving "+from.Name+": "+t.Why)
	if err := m.cfg.Move(subject, from.Name, t.To, t.Why); err != nil {
		m.record(Entry{Pass: pass, Subject: subject, State: from.Name, Step: StepFailed, Detail: "move", Err: err})
		return
	}
	m.record(Entry{Pass: pass, Subject: subject, State: from.Name, Step: StepMoved,
		Detail: t.On.EventName() + " -> " + t.To + ": " + t.Why})
	m.Wake(subject, "moved") // the new state may act at once; Look re-passes for itself
}

// cancel stops whatever is running for a subject and says why. Start, cancel and restart are three
// entries and not one silent replacement.
func (m *machine[W]) cancel(subject, pass, why string) {
	m.mu.Lock()
	in := m.act[subject]
	m.mu.Unlock()
	if in == nil {
		return
	}
	m.record(Entry{Pass: in.pass, Subject: subject, Step: StepCancelled, Detail: in.action + ": " + why})
	in.cancel()
}
