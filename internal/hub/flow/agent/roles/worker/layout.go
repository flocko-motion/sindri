// package: hub/flow/agent/roles/worker / layout
// type:    rendering (the worker flow's default drawing)
// job:     where the flow debug view draws each worker state by default.
// limits:  generated — arrange it in the debug view (URL: `sindri hub info`) with ?dev, then Export layout
// and replace this file. A state missing here is placed by dagre; a name that is no state fails a test.
package worker

import "github.com/flo-at/sindri/internal/hub/flow/machine"

// Layout is the worker flow's default drawing, by state name.
var Layout = machine.Layout{
	"worker/idle":             {X: 438, Y: -646},
	"worker/assigning":        {X: 927, Y: -1146},
	"worker/preparing":        {X: 1131, Y: -1135},
	"worker/retiering":        {X: 1318, Y: -1136},
	"worker/handing-over":     {X: 1515, Y: -1139},
	"worker/working":          {X: 1939, Y: -970},
	"worker/refreshing":       {X: 2436, Y: -724},
	"worker/reworking":        {X: 1774, Y: -900},
	"worker/interviewing":     {X: 2182, Y: -898},
	"worker/submitting":       {X: 2441, Y: -897},
	"worker/gating":           {X: 1756, Y: -737},
	"worker/submitted":        {X: 1912, Y: -741},
	"worker/resolving":        {X: 1756, Y: -971},
	"worker/between-subtasks": {X: 330, Y: -1000},
	"worker/picking-subtask":  {X: 985, Y: -1087},
	"worker/feature-gated":    {X: 658, Y: -1052},
	"worker/feature-done":     {X: 634, Y: -958},
	"worker/releasing":        {X: 468, Y: -852},
	"worker/yielding":         {X: 308, Y: -854},
	"worker/promoting":        {X: 2102, Y: -653},
	"worker/rebasing":         {X: 2164, Y: -1070},
	"worker/launching":        {X: 90, Y: -647},
	"worker/stopping":         {X: 80, Y: -800},
	"worker/clearing":         {X: 94, Y: -725},
	"worker/escalated":        {X: 83, Y: -983},
	"worker/retired":          {X: 92, Y: -1046},
	"worker/not-done":         {X: 83, Y: -889},
	"worker/stalled":          {X: 2166, Y: -958},
}
