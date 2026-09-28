// package: hub/flow/agent/roles/worker / layout
// type:    rendering (the worker flow's default drawing)
// job:     where the flow debug view draws each worker state by default.
// limits:  generated — arrange the graph in `sindri debug flow` opened with ?dev, then Export layout
// and replace this file. A state missing here is placed by dagre; a name that is no state fails a test.
package worker

import "github.com/flo-at/sindri/internal/hub/flow/machine"

// Layout is the worker flow's default drawing, by state name.
var Layout = machine.Layout{
	"worker/idle":             {X: 648, Y: -28},
	"worker/assigning":        {X: 1082, Y: -143},
	"worker/preparing":        {X: 1276, Y: -139},
	"worker/retiering":        {X: 1466, Y: -138},
	"worker/handing-over":     {X: 1672, Y: -136},
	"worker/working":          {X: 1556, Y: 679},
	"worker/refreshing":       {X: 1440, Y: 1577},
	"worker/reworking":        {X: 1863, Y: 1564},
	"worker/interviewing":     {X: 1092, Y: 1368},
	"worker/submitting":       {X: 859, Y: 1386},
	"worker/gating":           {X: 791, Y: 1585},
	"worker/submitted":        {X: 665, Y: 1383},
	"worker/resolving":        {X: 274, Y: 1393},
	"worker/between-subtasks": {X: 389, Y: 1171},
	"worker/picking-subtask":  {X: 210, Y: 769},
	"worker/feature-gated":    {X: -89, Y: 739},
	"worker/feature-done":     {X: -73, Y: 540},
	"worker/releasing":        {X: 372, Y: 536},
	"worker/yielding":         {X: 545, Y: 532},
	"worker/promoting":        {X: 574, Y: 1170},
	"worker/rebasing":         {X: 482, Y: 1384},
	"worker/launching":        {X: 328, Y: 161},
	"worker/stopping":         {X: 303, Y: -125},
	"worker/clearing":         {X: 655, Y: 204},
	"worker/escalated":        {X: 639, Y: -334},
	"worker/retired":          {X: 116, Y: 66},
	"worker/not-done":         {X: 868, Y: 144},
	"worker/stalled":          {X: 1915, Y: 643},
}
