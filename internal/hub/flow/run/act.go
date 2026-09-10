// package: hub/flow/run / act
// type:    logic (the actions the run map can run, as identities)
// job:     name every action the run flow attaches to a state, with the outcomes it can finish
// with — an identity and nothing more, so the map can name one without importing anything that
// writes.
// limits:  the names and the outcome vocabulary. The implementations are registered against these
// (-> hub/flow/run), and a build check fails on a declared action with none.
package run

// The outcomes a run's actions finish with. Named for what happened to the RUN, so a state's Events
// list reads as the run's own story rather than as the engine's.
var (
	Ran     = Outcome{Name: "ran"}     // the command ran and its verdict is on the record
	Dropped = Outcome{Name: "dropped"} // it was never worth running, and says so on the record
	Refused = Outcome{Name: "refused"} // the workspace, image or cache could not be prepared
)

// Execute runs the command to completion in a fresh capped container against a copy of its
// workspace, and records the verdict. Every ending is terminal: a run that finishes, one that times
// out and one that is killed all leave the row settled with its output on record.
var Execute = &Action{Name: "run-execute", Outcomes: []Outcome{Ran, Refused}}

// Drop settles a queued run nobody wants any more, writing WHY on the record rather than letting it
// disappear — a run that vanishes reads as one that was never queued.
var Drop = &Action{Name: "run-drop", Outcomes: []Outcome{Dropped}}

// Actions is every declared action, for the check that each has an implementation.
var Actions = []*Action{Execute, Drop}
