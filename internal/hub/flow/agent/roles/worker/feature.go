// package: hub/flow/agent/roles/worker / feature
// type:    logic (the worker's subtask loop, declared)
// job:     the three states a worker holding a whole feature passes through between its children —
// one to hand over, none left but gated, and none left at all.
// limits:  the map. A worker inside a subtask is in the ordinary working state: what it holds is a
// fact of the world, not a different way of working.
package worker

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
)

// The feature-holder's states.
const (
	Between      = "worker/between-subtasks"
	Picking      = "worker/picking-subtask"
	FeatureGated = "worker/feature-gated"
	FeatureDone  = "worker/feature-done"
)

// between: holding a feature and no child of it — the boundary every reset is taken at.
var between = flow.State{
	WhenIdle: flow.Nudge, // it holds the feature: the next subtask or the submit is its to get on with
	Name:     Between,
	In:       GroupFeatureHeld,
	Title:    "Between subtasks of a feature",
	About: "The worker holds a feature and none of its children. This is a LEAF BOUNDARY: nothing " +
		"is half done, so an armed clear fires here and the next subtask is claimed into a fresh " +
		"session.",
	Says: says.FeatureGated,
	Events: flow.Events{
		{cond.TreeSplit, Yielding, "another agent is working inside its tree — the container holder yields"},
		{cond.FeatureGone, Releasing, "the feature landed, or was closed, without it"},
		{cond.ClearArmed, Clearing, "a human armed a context clear — it fires at this boundary and no other"},
		// Straight to the claim, as an idle worker goes: the child is selected first and the session
		// prepared behind it. Nothing about the session is read HERE, because this is a state the
		// worker rests in, and a preparation condition read from rest is a way back into preparation.
		{cond.SubtaskReady, Picking, "the feature has an open child to hand over"},
		{cond.SubtasksGated, FeatureGated, "what is left awaits the user's verdict"},
		{cond.FeatureFinished, FeatureDone, "every child is closed"},
	},
	Verbs: flow.Offers{
		{verb.Task, "read the backlog"},
		{verb.Log, "record a note"},
		{verb.Mail, "read your mailbox"},
		{verb.Git, "read your changes"},
		{verb.Resolve, "check your branch still merges"},
		{verb.Rebase, "align onto the reference branch"},
		{verb.Escalate, "stop on a question"},
	},
}

// picking: the hub is moving the worker onto the feature's next child.
var picking = flow.State{
	WhenIdle: flow.LetItRest, // the hub is choosing
	Name:     Picking,
	In:       GroupClaim,
	Title:    "Being handed a subtask",
	Action:   act.PickSubtask,
	About: "The hub is putting the worker on its feature's next open child, and saying nothing about " +
		"it. The claim comes before the preparation, so the work is held while that runs — and the " +
		"worker is told at the end of the chain rather than part-way along it.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Preparing, "the subtask is claimed; its session comes next"},
		{act.Nothing, Between, "nothing open after all — look again"},
		// Same as the backlog claim: a child that cannot be started is a fault to be looked at, and
		// dropping back to the boundary would only bring the worker straight back here.
		{act.Failed, Escalated, "the subtask could not be started, so a human has to look"},
		{flow.Orphaned{}, Between, "the hub restarted mid-claim — nothing was claimed"},
	},
	Verbs: flow.Offers{{verb.Log, "record a note"}},
}

// featureGated: the tree is unfinished but nothing in it is claimable yet.
var featureGated = flow.State{
	WhenIdle: flow.LetItRest, // the user's approval opens what is left
	Name:     FeatureGated,
	In:       GroupFeatureHeld,
	Title:    "Feature waiting on approvals",
	About: "Every child left under this feature is still awaiting the user's verdict, so there is " +
		"nothing to work and nothing to submit. The approval gate opening is what moves this.",
	Says: says.FeatureGated,
	Events: flow.Events{
		{cond.FeatureGone, Releasing, "the feature landed or was closed"},
		{cond.SubtaskReady, Picking, "an approval opened the next child"},
		{cond.FeatureFinished, FeatureDone, "the gated children were closed rather than approved"},
	},
	Verbs: flow.Offers{
		{verb.Task, "read the backlog"},
		{verb.Log, "record a note"},
		{verb.Mail, "read your mailbox"},
		{verb.Fyi, "one note to the user"},
		{verb.Escalate, "stop on a question"},
	},
}

// featureDone: the tree is finished and the feature itself can go up.
var featureDone = flow.State{
	WhenIdle: flow.Nudge, // every subtask is checkpointed, so putting the branch up is its own next act
	Name:     FeatureDone,
	In:       GroupFeatureHeld,
	Title:    "Feature finished",
	About: "Every child of this feature is closed, so the feature itself is what goes up for review. " +
		"A feature goes up when its last subtask lands, not when somebody decides to cut it.",
	Says: says.FeatureDone,
	Events: flow.Events{
		{cond.FeatureGone, Releasing, "the feature landed or was closed"},
		{cond.SubtaskReady, Picking, "a child was reopened under it"},
	},
	Verbs: flow.Offers{
		{verb.Submit, "file the feature for review"},
		{verb.Git, "read your changes"},
		{verb.Log, "record a note"},
		{verb.Escalate, "stop on a question"},
	},
}
