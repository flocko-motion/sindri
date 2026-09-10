// package: hub/flow/roles/worker / worker
// type:    logic (the worker's flow, declared)
// job:     the map of how a worker works — every state it can be in, what the hub does there, what
// moves it out, what it may type, and what it is told. Read this file to know the worker.
// limits:  the map. Conditions are flow/cond's, actions are named in flow/act and implemented in the
// workflow, and the words behind each Says live there too.
package worker

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
	"github.com/flo-at/sindri/internal/hub/flow/agent/verb"
)

// The worker's states. Named here so the map below reads as one page rather than as forward
// references to four files.
const (
	Idle       = "worker/idle"
	Assigning  = "worker/assigning"
	Working    = "worker/working"
	Submitting = "worker/submitting"
	Gating     = "worker/gating"
	Submitted  = "worker/submitted"
	Resolving  = "worker/resolving"
	Clearing   = "worker/clearing"
	Retiering  = "worker/retiering"
	Escalated  = "worker/escalated"
	Retired    = "worker/retired"
)

// idle: holding nothing, watching for something to hold.
var idle = flow.State{
	Name:  Idle,
	Title: "Idle",
	About: "The worker holds no task and no feature. It may still be answerable for a PR it filed " +
		"before its state row was cleared, which is why that is watched here too.",
	Says: says.NoTasks,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question only the user can answer"},
		{cond.MailWaiting, Mail, "it has mail it has not read, so it is not done"},
		// A pull request of its own comes BEFORE retirement: answering for work it already filed is
		// not new work, and retirement stops new work. A retired author still owes its reviewer a
		// reply, and telling it to wait quietly instead is how a PR sat unanswered for four days.
		{cond.OwnPRRejected, Refreshing, "a PR of its own came back rejected"},
		{cond.OwnPROpen, Submitted, "a PR of its own is still to land"},
		{cond.Retired, Retired, "a human wound it down, and it holds nothing to answer for"},
		{cond.ClearArmed, Clearing, "a human armed a context clear — it fires before any claim"},
		{cond.HoldsFeature, Between, "it holds a feature, so the subtask loop answers for it"},
		{cond.WorkAvailable, Assigning, "the backlog has a unit rated for it"},
	},
	Verbs: flow.Offers{
		{verb.Next, Assigning, "take the next task"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Git, flow.Stay, "read your changes"},
		{verb.Resolve, flow.Stay, "check your branch still merges"},
		{verb.Rebase, flow.Stay, "align onto the reference branch"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

// assigning: the hub is handing work over, which takes time and can be cut off.
var assigning = flow.State{
	Name:   Assigning,
	Title:  "Being handed work",
	Action: act.PickWork,
	About: "The hub is claiming a unit for this worker — syncing the backlog, branching, and " +
		"preparing its session. The claim comes FIRST, so holding the work protects it while the " +
		"preparation behind it runs.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Working, "the work is claimed and the branch is laid down"},
		{cond.HoldsFeature, Between, "what it took was a feature, so its children come next"},
		{act.Nothing, Idle, "the backlog had nothing after all"},
		{act.Held, Idle, "a rule refused the claim for now"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-assignment — nothing was handed over"},
		{cond.Retired, Retired, "a human wound it down while the hand-over ran"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

// working: the agent is the actor here, not the hub.
var working = flow.State{
	Name:  Working,
	Title: "Working a task",
	About: "The worker holds work on its own branch and is inside it. The hub does nothing but " +
		"watch that the work still exists and still belongs to it.",
	Says: says.Working,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question only the user can answer"},
		{cond.MergeConflicted, Resolving, "the merge of its PR conflicts; the branch is back in its workspace"},
		{cond.MilestoneLanded, Rebasing, "a milestone of its own landed; its branch is behind that base"},
		{cond.GainedChildren, Promoting, "the task it holds grew children, so it holds a feature"},
		{cond.Rejected, Refreshing, "a verdict came back asking for another round"},
		{cond.Stalled, Stalled, "it holds this work and has stopped doing it"},
		{cond.BetweenSubtasks, Between, "it holds the feature and no child of it — a leaf boundary"},
		{cond.TaskGone, Idle, "the work it held was closed or given to somebody else"},
		{cond.FeatureGone, Releasing, "the feature it held landed without it"},
		{cond.TreeSplit, Yielding, "another agent is working inside its tree"},
	},
	Verbs: flow.Offers{
		{verb.Submit, Submitting, "file what you have for review"},
		{verb.Checkpoint, Submitting, "land an interim slice"},
		{verb.Run, flow.Stay, "queue a slow build or test"},
		{verb.Git, flow.Stay, "read your changes"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Comment, flow.Stay, "comment on the task"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Rebase, flow.Stay, "align onto the reference branch"},
		{verb.Resolve, flow.Stay, "check the branch still merges"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

// submitting: the hub is running the gate and filing the PR.
var submitting = flow.State{
	Name:   Submitting,
	Title:  "Submitting",
	Action: act.Submit,
	About: "The hub is taking what the worker has: the quality gate first, then a pull request. " +
		"A gate that queues hands the answer to the run queue rather than to this state.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Submitted, "the pull request is out"},
		{act.Queued, Gating, "the gate took it; its result decides"},
		{act.Failed, Refreshing, "the gate said no — its output is the next round's brief"},
		{flow.Orphaned{}, Working, "the hub restarted mid-submit — outcome unknown"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

// gating: somebody else's queue owns the answer.
var gating = flow.State{
	Name:  Gating,
	Title: "Queued at the quality gate",
	About: "The worker's commit is waiting on the fleet's single gate slot. The gate's own result " +
		"moves it on — to a filed PR, or back to the failure it has to answer.",
	Says: says.Gating,
	Events: flow.Events{
		{cond.OwnPROpen, Submitted, "the gate passed and the pull request is out"},
		{cond.Rejected, Refreshing, "the gate failed it"},
		{cond.TaskGone, Idle, "the work was closed under it"},
		{cond.Escalated, Escalated, "it stopped on a question"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}, {verb.Mail, flow.Stay, "read your mailbox"}},
}

// submitted: a reviewer owes it a verdict.
var submitted = flow.State{
	Name:  Submitted,
	Title: "Waiting on a verdict",
	About: "The pull request is filed and somebody else owes it a verdict. Nothing is asked of the " +
		"worker until that lands — and a rejection puts it back on the work, which is where the " +
		"feedback is answered.",
	Says: says.AwaitVerdict,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.MergeConflicted, Resolving, "the merge of its PR conflicts; the branch is back in its workspace"},
		{cond.Rejected, Refreshing, "the verdict came back rejected — the feedback is the brief"},
		{cond.OwnPRRejected, Refreshing, "its own PR came back rejected"},
		// Before the release, all three: a landing that leaves work in hand is not a release. A
		// milestone moved the base under a branch it still holds, a task that grew is still its own,
		// and a feature holder goes back to its children rather than to the backlog.
		{cond.MilestoneLanded, Rebasing, "a milestone of its own landed; its branch is behind that base"},
		{cond.GainedChildren, Promoting, "the task it holds grew children, so it holds a feature"},
		{cond.HoldsFeature, Between, "its feature is still in hand; the next child comes from there"},
		{cond.PRSettled, Idle, "the pull request landed or was withdrawn, and it holds nothing else"},
	},
	Verbs: flow.Offers{
		{verb.Revoke, Working, "withdraw the pull request"},
		{verb.Git, flow.Stay, "read your changes"},
		{verb.Resolve, flow.Stay, "check your branch still merges"},
		{verb.Rebase, flow.Stay, "align onto the reference branch"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

// resolving: a conflict is in the agent's hands.
var resolving = flow.State{
	Name:  Resolving,
	Title: "Resolving a conflict",
	About: "The merge hit a conflict and handed the branch back. The worker owns the resolution; " +
		"the hub watches only that the work it belongs to still exists.",
	Says: says.Resolving,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.TaskGone, Idle, "the work was closed under it"},
		// Leaving is the ABSENCE of the conflict, not the verb succeeding: an agent that fixed the
		// branch by hand is as resolved as one that ran `resolve`. What it holds is untouched — a
		// conflict is a state of the BRANCH, and resolving one loses nobody their work.
		{cond.NoConflict, Working, "nothing conflicts any more"},
	},
	Verbs: flow.Offers{
		{verb.Resolve, flow.Stay, "check the branch merges now"},
		{verb.Rebase, flow.Stay, "align onto the reference branch"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Submit, Submitting, "file it once it is clean"},
		{verb.Git, flow.Stay, "read your changes"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}
