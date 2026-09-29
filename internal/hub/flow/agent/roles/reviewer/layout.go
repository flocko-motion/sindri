// package: hub/flow/agent/roles/reviewer / layout
// type:    rendering (the reviewer flow's default drawing)
// job:     where the flow debug view draws each reviewer state by default.
// limits:  generated — arrange it in the debug view (URL: `sindri hub info`) with ?dev, then Export layout
// and replace this file. A state missing here is placed by dagre; a name that is no state fails a test.
package reviewer

import "github.com/flo-at/sindri/internal/hub/flow/machine"

// Layout is the reviewer flow's default drawing, by state name.
var Layout = machine.Layout{
	"reviewer/idle":      {X: 413, Y: 18},
	"reviewer/not-done":  {X: 186, Y: -222},
	"reviewer/taking":    {X: 741, Y: 231},
	"reviewer/reviewing": {X: 1138, Y: 46},
	"reviewer/dropping":  {X: 1174, Y: -54},
	"reviewer/launching": {X: 741, Y: 112},
	"reviewer/stopping":  {X: 31, Y: -88},
	"reviewer/clearing":  {X: 411, Y: 227},
	"reviewer/escalated": {X: 413, Y: -257},
	"reviewer/retired":   {X: 55, Y: 94},
}
