// package: hub/flow/agent/roles/coauthor / layout
// type:    rendering (the coauthor flow's default drawing)
// job:     where the flow debug view draws each coauthor state by default.
// limits:  generated — arrange the graph in `sindri debug flow` opened with ?dev, then Export layout
// and replace this file. A state missing here is placed by dagre; a name that is no state fails a test.
package coauthor

import "github.com/flo-at/sindri/internal/hub/flow/machine"

// Layout is the coauthor flow's default drawing, by state name.
var Layout = machine.Layout{
	"coauthor/collab":    {X: 349, Y: -46},
	"coauthor/launching": {X: 138, Y: -111},
	"coauthor/stopping":  {X: 573, Y: -122},
	"coauthor/clearing":  {X: 206, Y: 126},
	"coauthor/escalated": {X: 352, Y: -228},
	"coauthor/retired":   {X: 517, Y: 128},
}
