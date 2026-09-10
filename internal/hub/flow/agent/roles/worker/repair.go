// package: hub/flow/agent/roles/worker / repair
// type:    logic (the worker's repair states, declared)
// job:     the three places a worker goes when the world moved under it — a verdict came back, the
// feature landed without it, or somebody else got inside its tree. Each is a state because each
// takes a real action, not just a change of mind.
// limits:  the map. What each action does is hub/flow/fleet's (-> flowdo.go).
package worker

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
)

const (
	Refreshing = "worker/refreshing"
	Reworking  = "worker/reworking"
	Releasing  = "worker/releasing"
	Yielding   = "worker/yielding"
)

// refreshing: a new round starts on a fresh session, the same way a claim does.
var refreshing = flow.State{
	Name:   Refreshing,
	Title:  "Starting the next round fresh",
	Action: act.Clear,
	About: "A rejection is a NEW ROUND on the same work, and a round starts fresh like a claim does: " +
		"the reasoning that produced the rejected work is exactly what the feedback asks to be " +
		"reconsidered, and carrying it over is how the same answer comes back a second time.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Reworking, "the session is fresh; the feedback is the brief"},
		{act.Failed, Reworking, "the clear never landed — a crowded session beats withholding the feedback"},
		{flow.Orphaned{}, Reworking, "the hub restarted mid-clear"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

// reworking: the verdict came back and the feedback is the brief for this round.
var reworking = flow.State{
	Name:  Reworking,
	Title: "Answering a rejection",
	About: "A verdict came back asking for another round. The feedback is the brief, so it arrives " +
		"the moment the agent lands here — the session it would have asked from was just discarded " +
		"by refreshing, and the round number travels with it as the fact that changes the approach.",
	Says:  says.Rejected,
	Tells: true,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.TaskGone, Idle, "the work was closed under it"},
		{cond.NotRejected, Working, "the rejection was withdrawn or answered"},
	},
	Verbs: flow.Offers{
		{verb.Submit, Submitting, "file the next round"},
		{verb.Run, flow.Stay, "queue a slow build or test"},
		{verb.Git, flow.Stay, "read your changes"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Comment, flow.Stay, "comment on the task"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

// releasing: the feature landed without it, so the hold is dropped.
var releasing = flow.State{
	Name:   Releasing,
	Title:  "Letting go of a landed feature",
	Action: act.Release,
	About: "The feature this worker held is closed at its source, or was carried in by a merged pull " +
		"request. There is nothing left to work, so the hold is dropped and any PR still standing " +
		"against it is settled.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Idle, "the hold is dropped; back to the backlog"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-release"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

// yielding: somebody else is inside this tree, and the container holder is the one that gives way.
var yielding = flow.State{
	Name:   Yielding,
	Title:  "Yielding a split tree",
	Action: act.Yield,
	About: "This worker holds a container while another agent holds a task under it — a tree split " +
		"after the fact by reparenting, which no claim guard can cover. The CONTAINER holder yields, " +
		"since the leaf is the concrete work, and its unsettled pull request goes with the feature.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Idle, "the tree is somebody else's; back to the backlog"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-yield"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}
