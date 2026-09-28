// package: hub/flow/agent/roles/planner / layout
// type:    rendering (the planner flow's default drawing)
// job:     where the flow debug view draws each planner state by default.
// limits:  generated — arrange the graph in `sindri debug flow` opened with ?dev, then Export layout
// and replace this file. A state missing here is placed by dagre; a name that is no state fails a test.
package planner

import "github.com/flo-at/sindri/internal/hub/flow/machine"

// Layout is the planner flow's default drawing, by state name.
var Layout = machine.Layout{
	"planner/idle":      {X: 353, Y: 125},
	"planner/planning":  {X: 743, Y: 131},
	"planner/submitted": {X: 580, Y: -85},
	"planner/disowning": {X: 559, Y: 235},
	"planner/launching": {X: 187, Y: 318},
	"planner/stopping":  {X: 83, Y: 51},
	"planner/clearing":  {X: 548, Y: 402},
	"planner/escalated": {X: 266, Y: -64},
	"planner/retired":   {X: 87, Y: 185},
}
