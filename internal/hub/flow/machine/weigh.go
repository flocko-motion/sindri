// package: hub/flow/machine / weigh
// type:    logic (a subject's exits, read against the world now)
// job:     report where a subject stands and which of that state's events hold on a world gathered
// now — the reading a pass would act on, taken without acting.
// limits:  reading. Nothing is moved, started or recorded; what a view makes of it is the view's.
package machine

// Weigh is where a subject stands and, per event of that state in declaration order, whether it
// holds now. An outcome never does: it arrives with its action and is never read off the world.
func (m *machine[W]) Weigh(subject string) (State[W], []bool, error) {
	s, since, err := m.standing(subject)
	if err != nil {
		return State[W]{}, nil, err
	}
	w, err := m.cfg.Gather(subject)
	if err != nil {
		return s, nil, err
	}
	holds := make([]bool, len(s.Events))
	for i, t := range s.Events {
		switch c := t.On.(type) {
		case Condition[W]:
			holds[i] = c.Holds != nil && c.Holds(w)
		case Orphaned:
			holds[i] = m.orphaned(subject, s, since)
		}
	}
	return s, holds, nil
}
