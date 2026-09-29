// package: hub/flow/machine / exits
// type:    logic (a state's exits, inherited from the groups it sits in)
// job:     resolve every state's effective exits — its own and its groups' — in the one order the
// engine checks them, and hold a flow's groups to the rules that keep inheritance sound.
// limits:  the resolution and its checks. Evaluating an exit is loop.go's; drawing one is graph.go's.
package machine

import "fmt"

// Exit is one of a state's effective exits: the transition, and who declared it — the state itself,
// or a group it sits in — with its place in that owner's own list.
type Exit[W any] struct {
	Transition[W]
	Owner string
	First bool // from a group's First slot (Owner is a group)
	Index int
}

// Exits is s's effective exits in the order a pass checks them: the outermost group's First down to
// the innermost's, the state's own, then the innermost group's Then out to the outermost's. Nesting
// reads as wrapping — a region's first word comes before everything inside it, its last after.
func Exits[W any](groups map[string]Group[W], s State[W]) []Exit[W] {
	var chain []Group[W] // innermost first
	for in := s.In; in != ""; {
		g, ok := groups[in]
		if !ok {
			break // an undeclared parent is checkGroups' to report
		}
		chain = append(chain, g)
		in = g.In
	}
	var out []Exit[W]
	for i := len(chain) - 1; i >= 0; i-- {
		for j, t := range chain[i].First {
			out = append(out, Exit[W]{Transition: t, Owner: chain[i].Name, First: true, Index: j})
		}
	}
	for j, t := range s.Events {
		out = append(out, Exit[W]{Transition: t, Owner: s.Name, Index: j})
	}
	for _, g := range chain {
		for j, t := range g.Then {
			out = append(out, Exit[W]{Transition: t, Owner: g.Name, Index: j})
		}
	}
	return out
}

// transitions is just the transitions of a state's effective exits, for the loop's readers.
func transitions[W any](exits []Exit[W]) []Transition[W] {
	out := make([]Transition[W], len(exits))
	for i, e := range exits {
		out[i] = e.Transition
	}
	return out
}

// checkGroups holds the groups to what makes inheritance sound: names unique and apart from the
// states', every parent declared and no region inside itself, only conditions in a group, and every
// state's In naming a group that exists.
func (m *machine[W]) checkGroups() error {
	for name, g := range m.groups {
		if _, clash := m.states[name]; clash {
			return fmt.Errorf("machine: %q is declared as both a state and a group", name)
		}
		if g.Title == "" || g.About == "" {
			return fmt.Errorf("machine: group %q must declare a Title and an About", name)
		}
		seen := map[string]bool{name: true}
		for in := g.In; in != ""; in = m.groups[in].In {
			if _, ok := m.groups[in]; !ok {
				return fmt.Errorf("machine: group %q sits in undeclared %q", name, in)
			}
			if seen[in] {
				return fmt.Errorf("machine: group %q sits inside itself", name)
			}
			seen[in] = true
		}
		for _, t := range append(append([]Transition[W]{}, g.First...), g.Then...) {
			switch t.On.(type) {
			case Outcome, Orphaned:
				return fmt.Errorf("machine: group %q declares %q — an outcome or an orphan belongs to one state's own action", name, t.On.EventName())
			}
			if _, ok := m.states[t.To]; !ok && t.To != Stay {
				return fmt.Errorf("machine: group %q leads to undeclared %q", name, t.To)
			}
		}
	}
	for _, s := range m.states {
		if _, ok := m.groups[s.In]; s.In != "" && !ok {
			return fmt.Errorf("machine: state %q sits in undeclared group %q", s.Name, s.In)
		}
	}
	return nil
}
