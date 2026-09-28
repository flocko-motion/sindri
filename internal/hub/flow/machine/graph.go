// package: hub/flow/machine / graph
// type:    rendering (a declared flow, as data)
// job:     export any flow's declarations as the wire graph the debug view draws — every state, every
// edge in declaration order with its kind, and what each state runs and offers.
// limits:  rendering, for every machine alike. Where a subject stands is Weigh's; which flow a
// subject runs is its engine's.
package machine

import (
	"fmt"

	"github.com/flo-at/sindri/internal/api"
)

// Pos is where the debug view draws a state, in the graph's own coordinates.
type Pos struct{ X, Y int }

// Layout is a flow's default drawing, by state name — exported from the debug view by a developer
// and checked in beside the flow; a state it does not name is placed by the view's own layout.
type Layout map[string]Pos

// Graph exports declared states as the wire graph, in declaration order.
func Graph[W any](states []State[W]) []api.FlowState {
	out := make([]api.FlowState, 0, len(states))
	for _, s := range states {
		out = append(out, graphState(s))
	}
	return out
}

func graphState[W any](s State[W]) api.FlowState {
	out := api.FlowState{Name: s.Name, Title: s.Title, About: s.About, Says: s.Says, Tells: s.Tells,
		WhenIdle: idleWord(s.WhenIdle), Events: make([]api.FlowEvent, 0, len(s.Events))}
	if s.Action != nil {
		out.Action, out.Awaits = s.Action.Name, s.Action.Awaits
		for _, o := range s.Action.Outcomes {
			out.Outcomes = append(out.Outcomes, o.Name)
		}
	}
	for _, t := range s.Events {
		ev := api.FlowEvent{On: t.On.EventName(), To: t.To, Why: t.Why}
		switch c := t.On.(type) {
		case Condition[W]:
			ev.Kind, ev.Within = "condition", c.Within.String()
			for _, w := range c.Wake {
				ev.Wake = append(ev.Wake, string(w))
			}
		case Outcome:
			ev.Kind = "outcome"
		case Orphaned:
			ev.Kind = "orphaned"
		default:
			ev.Kind = fmt.Sprintf("%T", t.On) // a new event kind shows up named, never as a blank
		}
		out.Events = append(out.Events, ev)
	}
	for _, v := range s.Verbs {
		out.Verbs = append(out.Verbs, api.FlowVerb{Verb: v.Verb.Name, Why: v.Why})
	}
	return out
}

func idleWord(w WhenIdle) string {
	switch w {
	case LetItRest:
		return "rest"
	case Nudge:
		return "nudge"
	}
	return "undeclared"
}
