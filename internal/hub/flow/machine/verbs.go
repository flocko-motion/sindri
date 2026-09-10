// package: hub/flow/machine / verbs
// type:    logic (what a subject may run where it stands)
// job:     offer the verbs its current state declares, refuse the rest by name with that list, and
// move the subject where the verb it ran says it leads.
// limits:  offering and routing. What a verb DOES is the domain's, run through the state it leads to.
package machine

import (
	"context"
	"fmt"
	"strings"
)

// Verbs is what a subject may run where it stands.
func (m *machine[W]) Verbs(subject string) ([]Offer, error) {
	s, err := m.State(subject)
	if err != nil {
		return nil, err
	}
	return append(make([]Offer, 0, len(s.Verbs)), s.Verbs...), nil
}

// Run performs a verb for a subject. A verb the state does not offer is refused by name, WITH what
// is open instead: a refusal that cannot say what to reach for leaves the caller to guess the flow.
func (m *machine[W]) Run(ctx context.Context, subject, verb string) (Offer, error) {
	pass := m.passes.next()
	s, err := m.State(subject)
	if err != nil {
		return Offer{}, err
	}
	for _, o := range s.Verbs {
		if o.Verb.Name != verb {
			continue
		}
		m.record(Entry{Pass: pass, Subject: subject, State: s.Name, Step: StepVerb, Detail: verb})
		if o.To != Stay {
			m.move(pass, subject, s, Transition[W]{On: Outcome{Name: "verb " + verb}, To: o.To, Why: o.Why})
		}
		return o, nil
	}
	m.record(Entry{Pass: pass, Subject: subject, State: s.Name, Step: StepVerb, Detail: "refused " + verb})
	return Offer{}, fmt.Errorf("%s is not available in %s — available here: %s", verb, s.Name, offered(s))
}

// offered names what a state does allow, for the refusal to point at.
func offered[W any](s State[W]) string {
	if len(s.Verbs) == 0 {
		return "nothing"
	}
	names := make([]string, 0, len(s.Verbs))
	for _, o := range s.Verbs {
		names = append(names, o.Verb.Name)
	}
	return strings.Join(names, ", ")
}
