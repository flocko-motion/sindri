// package: hub/flow/agent/idle / idle
// type:    logic (what a still screen means, read off the maps)
// job:     answer, for a phase, whether an agent doing nothing there is a fault — the flows' own
// per-state declaration, served to the sweep that prods and the board word that reports it.
// limits:  the reading. The declaration is each state's (-> machine.State.WhenIdle), and a LEAF
// package so the fixtures can reach it without importing the acting half they are fixtures for.
package idle

import (
	"time"

	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/coauthor"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/planner"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/reviewer"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
)

// byPhase indexes every state of every role. Names are unique across the four flows, so one map
// answers for all of them — the same property the machine relies on to run over them at once.
var byPhase = func() map[string]flow.State {
	m := map[string]flow.State{}
	for _, f := range [][]flow.State{worker.Flow, planner.Flow, reviewer.Flow, coauthor.Flow} {
		for _, s := range f {
			m[s.Name] = s
		}
	}
	return m
}()

// Rule reports whether an agent standing in this phase should be prodded for doing nothing, and how
// long a still screen is tolerated first. An unknown phase rests: nothing declared says otherwise.
func Rule(phase string) (after time.Duration, nudge bool) {
	s, ok := byPhase[phase]
	if !ok || s.WhenIdle != flow.Nudge {
		return 0, false
	}
	return s.NudgeAfter, true
}
