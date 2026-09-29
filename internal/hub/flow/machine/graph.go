// package: hub/flow/machine / graph
// type:    rendering (a declared flow, as data)
// job:     export any flow's declarations as the wire graph the debug view draws — every state and
// group, every edge in declaration order with its kind, each state's effective exits in checking
// order, and what each state runs and offers.
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

// Graph exports declared states and groups as the wire graph, in declaration order.
func Graph[W any](states []State[W], groups []Group[W]) ([]api.FlowState, []api.FlowGroup) {
	byName := make(map[string]Group[W], len(groups))
	outGroups := make([]api.FlowGroup, 0, len(groups))
	for _, g := range groups {
		byName[g.Name] = g
		outGroups = append(outGroups, api.FlowGroup{Name: g.Name, Title: g.Title, About: g.About, In: g.In,
			First: flowEvents(g.First), Then: flowEvents(g.Then)})
	}
	out := make([]api.FlowState, 0, len(states))
	for _, s := range states {
		out = append(out, graphState(byName, s))
	}
	return out, outGroups
}

func graphState[W any](groups map[string]Group[W], s State[W]) api.FlowState {
	out := api.FlowState{Name: s.Name, Title: s.Title, About: s.About, In: s.In, Says: s.Says, Tells: s.Tells,
		WhenIdle: idleWord(s.WhenIdle), Events: flowEvents(s.Events)}
	if s.Action != nil {
		out.Action, out.Awaits = s.Action.Name, s.Action.Awaits
		for _, o := range s.Action.Outcomes {
			out.Outcomes = append(out.Outcomes, o.Name)
		}
	}
	for _, e := range Exits(groups, s) {
		out.Exits = append(out.Exits, api.FlowExitRef{Owner: e.Owner, First: e.First, Index: e.Index})
	}
	for _, v := range s.Verbs {
		out.Verbs = append(out.Verbs, api.FlowVerb{Verb: v.Verb.Name, Why: v.Why})
	}
	return out
}

func flowEvents[W any](ts []Transition[W]) []api.FlowEvent {
	out := make([]api.FlowEvent, 0, len(ts))
	for _, t := range ts {
		ev := api.FlowEvent{On: t.On.EventName(), To: t.To, Why: t.Why, Kind: string(t.KindOf())}
		switch c := t.On.(type) {
		case Condition[W]:
			ev.Trigger, ev.Within = "condition", c.Within.String()
			for _, w := range c.Wake {
				ev.Wake = append(ev.Wake, string(w))
			}
		case Outcome:
			ev.Trigger = "outcome"
		case Orphaned:
			ev.Trigger = "orphaned"
		default:
			ev.Trigger = fmt.Sprintf("%T", t.On) // a new event type shows up named, never as a blank
		}
		out = append(out, ev)
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
