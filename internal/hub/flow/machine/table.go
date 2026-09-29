// package: hub/flow/machine / table
// type:    rendering (a declared flow, as text)
// job:     render a flow as the map a reader answers "what happens here" from — each state's title,
// what it means, what the hub does there, every way out, and what may be typed.
// limits:  rendering. What the states are is the flow's own declaration (-> State).
package machine

import (
	"fmt"
	"strings"
)

// Table renders declared states as text. It is the reason the declarations are data: a flow you can
// print is one a reader does not have to reconstruct from branches.
func Table[W any](states []State[W]) string {
	var b strings.Builder
	for i, s := range states {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s — %s\n", s.Name, s.Title)
		for _, line := range strings.Split(wrap(s.About, 92), "\n") {
			fmt.Fprintf(&b, "    %s\n", line)
		}
		if s.Action != nil {
			fmt.Fprintf(&b, "    runs %s\n", s.Action.Name)
		}
		for _, t := range s.Events {
			fmt.Fprintf(&b, "    on %-20s -> %-26s %s\n", t.On.EventName(), t.To, t.Why)
		}
		for _, v := range s.Verbs {
			fmt.Fprintf(&b, "    verb %-18s %s\n", v.Verb.Name, v.Why)
		}
	}
	return b.String()
}

// wrap breaks s onto lines of at most width runes, at spaces.
func wrap(s string, width int) string {
	var b strings.Builder
	col := 0
	for i, w := range strings.Fields(s) {
		switch {
		case i == 0:
		case col+1+len(w) > width:
			b.WriteString("\n")
			col = 0
		default:
			b.WriteString(" ")
			col++
		}
		b.WriteString(w)
		col += len(w)
	}
	return b.String()
}
