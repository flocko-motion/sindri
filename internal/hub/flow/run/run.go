// package: hub/flow/run / run
// type:    logic (the run queue's flow, declared)
// job:     the map of how a queued command becomes a verdict — every state a run can be in, what
// the hub does there, what moves it out, and where each ending leads. Read this file to know the
// run queue.
// limits:  the map. The conditions are cond.go's, the actions are named in act.go and implemented
// in run_act.go, and the queue's ranking is there too.
package run

// The run's states. A run is not an actor, so it types nothing and is told nothing — the map is
// only about what the HUB does with it.
const (
	Queued    = "run/queued"
	Executing = "run/executing"
	Dropping  = "run/dropping"
	Finished  = "run/finished"
)

// queued: waiting on the fleet's one slot.
var queued = State{
	Name:  Queued,
	Title: "Queued",
	About: "The command is recorded and waiting. The queue is ONE slot across the whole fleet, " +
		"ranked together and never per project, so a run leaves here only when it leads that queue " +
		"and nothing else is executing.",
	Events: Events{
		{Settled, Finished, "a human withdrew it, or something else settled it, before it ran"},
		{Stale, Dropping, "the agent that asked for it has gone, or moved on to other work"},
		{AtFront, Executing, "it leads the queue and the slot is free"},
	},
}

// executing: the command is running, and the hub is holding the slot.
var executing = State{
	Name:   Executing,
	Title:  "Executing",
	Action: Execute,
	About: "The command is running in a fresh capped container against a COPY of the workspace it " +
		"was aimed at, so the live checkout is never at risk. It holds the fleet's only slot for as " +
		"long as it runs, which is what the hard cap exists to bound.",
	Events: Events{
		{Ran, Finished, "the command ran and its verdict is on the record"},
		{Refused, Finished, "the workspace, image or cache could not be prepared"},
		{Orphaned{}, Finished, "the hub restarted mid-run — outcome unknown, and its container is gone"},
	},
}

// dropping: settling a run nobody wants any more, with the reason written down.
var dropping = State{
	Name:   Dropping,
	Title:  "Being dropped",
	Action: Drop,
	About: "The run is being settled without executing, because the agent behind it has gone or has " +
		"moved on. The reason goes on the record: a run that simply vanished would read as one that " +
		"was never queued at all.",
	Events: Events{
		{Dropped, Finished, "the reason it was never run is on its record"},
		{Orphaned{}, Finished, "the hub restarted mid-drop"},
	},
}

// finished: the row carries its own verdict, and nothing watches it any more.
var finished = State{
	Name:  Finished,
	Title: "Finished",
	About: "The run is over and its row carries the verdict — passed, failed, timed out, cancelled. " +
		"NO way out, and none needed: a verdict already given is not revisited, and the queue stops " +
		"listing the run at all, so nothing looks here twice.",
}

// Flow is the run queue's whole map, in the order a reader should meet it.
var Flow = []State{queued, executing, dropping, finished}

// Start is where a run with nothing stored begins. Every run is queued before it is anything else.
const Start = Queued
