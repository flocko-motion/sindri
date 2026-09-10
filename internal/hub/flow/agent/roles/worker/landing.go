// package: hub/flow/roles/worker / landing
// type:    logic (what a landing does to the worker that filed it, declared)
// job:     the two states a worker passes through when a merge lands underneath it — the task it
// held having grown into a feature, and its standing branch needing to catch up with the base its
// own milestone moved.
// limits:  the map. Merging is the pull request's own business; these are what a merge MEANS for
// whoever filed it, which is this agent's to decide and not the merge's to write.
package worker

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
	"github.com/flo-at/sindri/internal/hub/flow/agent/verb"
)

const (
	Promoting = "worker/promoting"
	Rebasing  = "worker/rebasing"
)

// promoting: the leaf it holds has grown children, so it holds a feature.
var promoting = flow.State{
	Name:   Promoting,
	Title:  "Taking on work its task gained",
	Action: act.Promote,
	About: "The task this worker holds gained children while it was working, so its unit of work is " +
		"a FEATURE now. It takes the new work on rather than being stranded in front of it — and the " +
		"promotion happens on the one path that resumes an agent inside a feature, so nothing ends " +
		"up beside it.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Between, "it holds a feature now; its children come next"},
		{act.Failed, Working, "the promotion did not take — carry on with the leaf"},
		{flow.Orphaned{}, Working, "the hub restarted mid-promotion"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

// rebasing: a milestone of its own landed, so its standing branch is behind.
var rebasing = flow.State{
	Name:   Rebasing,
	Title:  "Catching up with its own milestone",
	Action: act.Rebase,
	About: "An interim pull request this worker filed merged, and its standing branch is behind the " +
		"base that merge moved. The branch is RESET onto it rather than rebased, keeping whatever is " +
		"mid-edit: a squashed merge makes the old commits unrecognisable, and discarding uncommitted " +
		"work to tidy that up would throw away the thing being worked on.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Working, "the branch is on the new base with its work intact"},
		{act.Failed, Resolving, "reapplying the uncommitted work conflicts — the agent resolves it"},
		{flow.Orphaned{}, Working, "the hub restarted mid-reset"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}
