// package: hub/flow/roles/worker / feature
// type:    logic (the worker's subtask loop, declared)
// job:     the three states a worker holding a whole feature passes through between its children —
// one to hand over, none left but gated, and none left at all.
// limits:  the map. A worker inside a subtask is in the ordinary working state: what it holds is a
// fact of the world, not a different way of working.
package worker

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
	"github.com/flo-at/sindri/internal/hub/flow/agent/verb"
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
	Name:  Between,
	Title: "Between subtasks of a feature",
	About: "The worker holds a feature and none of its children. This is a LEAF BOUNDARY: nothing " +
		"is half done, so an armed clear fires here and the next subtask is claimed into a fresh " +
		"session.",
	Says: says.FeatureGated,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.TreeSplit, Yielding, "another agent is working inside its tree — the container holder yields"},
		{cond.FeatureGone, Releasing, "the feature landed, or was closed, without it"},
		{cond.ClearArmed, Clearing, "a human armed a context clear — it fires at this boundary and no other"},
		{cond.SubtaskReady, Picking, "the feature has an open child to hand over"},
		{cond.SubtasksGated, FeatureGated, "what is left awaits the user's verdict"},
		{cond.FeatureFinished, FeatureDone, "every child is closed"},
	},
	Verbs: flow.Offers{
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Git, flow.Stay, "read your changes"},
		{verb.Resolve, flow.Stay, "check your branch still merges"},
		{verb.Rebase, flow.Stay, "align onto the reference branch"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

// picking: the hub is moving the worker onto the feature's next child.
var picking = flow.State{
	Name:   Picking,
	Title:  "Being handed a subtask",
	Action: act.PickSubtask,
	About: "The hub is putting the worker on its feature's next open child, preparing the session " +
		"first. The claim comes before the preparation, so the work is held while that runs.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Working, "the subtask is claimed"},
		{act.Nothing, Between, "nothing open after all — look again"},
		{flow.Orphaned{}, Between, "the hub restarted mid-hand-over"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

// featureGated: the tree is unfinished but nothing in it is claimable yet.
var featureGated = flow.State{
	Name:  FeatureGated,
	Title: "Feature waiting on approvals",
	About: "Every child left under this feature is still awaiting the user's verdict, so there is " +
		"nothing to work and nothing to submit. The approval gate opening is what moves this.",
	Says: says.FeatureGated,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.FeatureGone, Releasing, "the feature landed or was closed"},
		{cond.SubtaskReady, Picking, "an approval opened the next child"},
		{cond.FeatureFinished, FeatureDone, "the gated children were closed rather than approved"},
	},
	Verbs: flow.Offers{
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

// featureDone: the tree is finished and the feature itself can go up.
var featureDone = flow.State{
	Name:  FeatureDone,
	Title: "Feature finished",
	About: "Every child of this feature is closed, so the feature itself is what goes up for review. " +
		"A feature goes up when its last subtask lands, not when somebody decides to cut it.",
	Says: says.FeatureDone,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.FeatureGone, Releasing, "the feature landed or was closed"},
		{cond.SubtaskReady, Picking, "a child was reopened under it"},
	},
	Verbs: flow.Offers{
		{verb.Submit, Submitting, "file the feature for review"},
		{verb.Git, flow.Stay, "read your changes"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}
