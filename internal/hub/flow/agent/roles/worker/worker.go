// package: hub/flow/agent/roles/worker / worker
// type:    logic (the worker's flow, declared)
// job:     the map of how a worker works — every state it can be in, what the hub does there, what
// moves it out, what it may type, and what it is told. Read this file to know the worker.
// limits:  the map. Conditions are flow/cond's, actions are named in flow/act and implemented in the
// workflow, and the words behind each Says live there too.
package worker

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
)

// The worker's states. Named here so the map below reads as one page rather than as forward
// references to four files.
const (
	Idle         = "worker/idle"
	Assigning    = "worker/assigning"
	HandingOver  = "worker/handing-over"
	Working      = "worker/working"
	Interviewing = "worker/interviewing"
	Submitting   = "worker/submitting"
	Gating       = "worker/gating"
	Submitted    = "worker/submitted"
	Resolving    = "worker/resolving"
)

// idle: holding nothing, watching for something to hold.
var idle = flow.State{
	WhenIdle: flow.LetItRest, // nothing has been selected for it yet
	Name:     Idle,
	Title:    "Idle",
	About: "The worker holds no task and no feature. It may still be answerable for a PR it filed " +
		"before its state row was cleared, which is why that is watched here too.",
	Says: says.NoTasks,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question only the user can answer"},
		// A pull request of its own comes BEFORE retirement: answering for work it already filed is
		// not new work, and retirement stops new work. A retired author still owes its reviewer a
		// reply, and telling it to wait quietly instead is how a PR sat unanswered for four days.
		{cond.OwnPRRejected, Refreshing, "a PR of its own came back rejected"},
		{cond.OwnPROpen, Submitted, "a PR of its own is still to land"},
		{cond.Retired, Retired, "a human wound it down, and it holds nothing to answer for"},
		// BEFORE the claim it refuses, which is the whole of what this standing means.
		{cond.NotDone, NotDone, "its mail is unread, so it is not done and takes nothing new"},
		{cond.ClearArmed, Clearing, "a human armed a context clear — it fires before any claim"},
		{cond.HoldsFeature, Between, "it holds a feature, so the subtask loop answers for it"},
		// Straight to the claim. The session is prepared BEHIND it, in the states the claim leads to —
		// nothing is checked here first, because a preparation condition read from a RESTING state is
		// a way back into preparation, which is how a worker came to circle for four days.
		{cond.WorkAvailable, Assigning, "the backlog has a unit rated for it"},
		// The pod comes LAST, after everything the agent could be answering for: a stop must not
		// interrupt an agent about to be handed work, and a start is only worth making for work
		// nobody awake would take.
		{cond.StartAsked, Launching, "a human asked for this pod"},
		{cond.NeededWhileAsleep, Launching, "the backlog has work, this pod was reclaimed, and nobody awake would take it"},
		{cond.StopAsked, Stopping, "a human asked for this pod back"},
		{cond.Reclaimable, Stopping, "it has held nothing long enough that its pod is worth taking back"},
	},
	Verbs: flow.Offers{
		{verb.Task, "read the backlog"},
		{verb.Git, "read your changes"},
		{verb.Resolve, "check your branch still merges"},
		{verb.Rebase, "align onto the reference branch"},
		{verb.Mail, "read your mailbox"},
		{verb.Log, "record a note"},
		{verb.Fyi, "one note to the user"},
		{verb.Escalate, "stop on a question"},
	},
}

// assigning: the hub is handing work over, which takes time and can be cut off.
var assigning = flow.State{
	WhenIdle: flow.LetItRest, // the hub is choosing
	Name:     Assigning,
	Title:    "Being handed work",
	Action:   act.PickWork,
	About: "The hub is claiming a unit for this worker — syncing the backlog and branching. The " +
		"claim comes FIRST and says nothing: holding the work protects it while the session is " +
		"prepared behind it, and the worker hears once, at the hand-over that ends the chain.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Preparing, "the work is claimed and the branch is laid down; its session comes next"},
		{act.Nothing, Idle, "the backlog had nothing after all"},
		// A selection that cannot be made is a fault in the repo or the store, not a turn the flow
		// takes: retrying it is how a worker span here for an hour, once every two seconds, while the
		// project's main checkout sat on a detached HEAD and nobody was told.
		{act.Failed, Escalated, "the claim could not be made, so a human has to look"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-assignment — nothing was claimed"},
		{cond.Retired, Retired, "a human wound it down while the claim ran"},
	},
	Verbs: flow.Offers{{verb.Log, "record a note"}},
}

// handingOver: the worker is told what it holds. The last step of the chain, and the only one it
// sees — the hub selects, prepares, and only then instructs.
var handingOver = flow.State{
	WhenIdle: flow.LetItRest, // the hub is speaking
	Name:     HandingOver,
	Title:    "Being handed its work",
	Action:   act.HandOver,
	About: "The work is claimed and the session is prepared, so the hub says what the worker now " +
		"holds. Everything before this was done TO the agent without its knowledge, which is what " +
		"lets the preparation run without a half-instructed session in the middle of it.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Working, "it has been told what it holds"},
		{act.Failed, Working, "the brief did not land; it holds the work and is told on its next ask"},
		{cond.SessionGone, Working, "the pod went away before the brief landed"},
		{flow.Orphaned{}, Working, "the hub restarted mid-hand-over, and the claim stands"},
	},
	Verbs: flow.Offers{{verb.Log, "record a note"}},
}

// working: the agent is the actor here, not the hub.
var working = flow.State{
	WhenIdle: flow.Nudge, // it holds the work and is inside it
	Name:     Working,
	Title:    "Working a task",
	About: "The worker holds work on its own branch and is inside it. The hub does nothing but " +
		"watch that the work still exists and still belongs to it.",
	Says: says.Working,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question only the user can answer"},
		// SECOND, under the escalation alone: this state exists to hold an agent AT work, and every
		// exit below it reads the work it is assumed to hold. An agent that has none is not working,
		// however it arrived — an escalation raised before anything was claimed resolves to here.
		{cond.HoldsNothing, Idle, "it holds neither a task nor a feature, so it is not at work"},
		{cond.MergeConflicted, Resolving, "the merge of its PR conflicts; the branch is back in its workspace"},
		{cond.MilestoneLanded, Rebasing, "a milestone of its own landed; its branch is behind that base"},
		{cond.GainedChildren, Promoting, "the task it holds grew children, so it holds a feature"},
		{cond.Rejected, Refreshing, "a verdict came back asking for another round"},
		{cond.Stalled, Stalled, "it holds this work and has stopped doing it"},
		{cond.SubmitAsked, Interviewing, "it asked to submit, and answers for the tree before it goes up"},
		{cond.BetweenSubtasks, Between, "it holds the feature and no child of it — a leaf boundary"},
		{cond.TaskGone, Idle, "the work it held was closed or given to somebody else"},
		{cond.FeatureGone, Releasing, "the feature it held landed without it"},
		{cond.TreeSplit, Yielding, "another agent is working inside its tree"},
		{cond.AsleepHolding, Launching, "it holds this work and its pod is gone — a claim must not outlive the pod it was made for"},
		// LAST, so every reason to stay at the work wins over it. An interim contribution is the case:
		// the author keeps the task and the branch, and there is nothing for it to do until the user
		// lands what it put up, since the branch it would carry on is the one waiting.
		{cond.AwaitingContribution, Submitted, "a contribution of its own is out, and it is the branch it would carry on"},
	},
	Verbs: flow.Offers{
		{verb.Submit, "file what you have for review"},
		{verb.Checkpoint, "land an interim slice"},
		{verb.Run, "queue a slow build or test"},
		{verb.Git, "read your changes"},
		{verb.Task, "read the backlog"},
		{verb.Log, "record a note"},
		{verb.Fyi, "one note to the user"},
		{verb.Comment, "comment on the task"},
		{verb.Mail, "read your mailbox"},
		{verb.Rebase, "align onto the reference branch"},
		{verb.Resolve, "check the branch still merges"},
		{verb.Escalate, "stop on a question"},
	},
}

// interviewing: the worker is answering for the tree it wants taken.
var interviewing = flow.State{
	WhenIdle: flow.Nudge, // the hub asked it a question and is waiting on the answer
	Name:     Interviewing,
	Title:    "Answering for a submit",
	Action:   act.Interview,
	About: "The worker asked to submit and the hub is putting its questions, one at a time, waiting " +
		"as long as each answer takes. Nothing is committed yet: the answers describe the tree as it " +
		"stands, so an author that edits it is answering about a tree that is gone and starts again.",
	Says: says.Interviewing,
	Events: flow.Events{
		{act.Done, Submitting, "every question is answered; what it holds can go up"},
		{act.Stale, Working, "the tree moved under the questions, so the answers describe nothing"},
		{act.Failed, Working, "the interview could not be conducted; the work is still in hand"},
		{cond.Escalated, Escalated, "it stopped on a question only the user can answer"},
		{cond.TaskGone, Idle, "the work it was submitting was closed or given to somebody else"},
		{cond.AsleepHolding, Launching, "its pod is gone, and nothing can be asked of a dead pane"},
		{cond.NoSubmitAsked, Working, "the submit it was answering for is gone"},
		{flow.Orphaned{}, Working, "the hub restarted mid-interview; the request stands and puts it back here"},
	},
	Verbs: flow.Offers{
		{verb.Submit, "answer the question standing against your submit"},
		{verb.Git, "read your changes"},
		{verb.Log, "record a note"},
		{verb.Escalate, "stop on a question"},
	},
}

// submitting: the hub is running the gate and filing the PR.
var submitting = flow.State{
	WhenIdle: flow.LetItRest, // the hub is filing what it has
	Name:     Submitting,
	Title:    "Submitting",
	Action:   act.Submit,
	About: "The hub is taking what the worker has: the commit first, then the quality gate, then a " +
		"pull request. A gate that queues hands the answer to the run queue rather than to this state.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Queued, Gating, "the gate took it; its result decides"},
		{act.Done, Submitted, "the gate had already passed this commit, and the pull request is out"},
		{act.Failed, Working, "the submit could not be taken; the work is still in hand"},
		{flow.Orphaned{}, Working, "the hub restarted mid-submit — outcome unknown"},
	},
	Verbs: flow.Offers{{verb.Log, "record a note"}},
}

// gating: somebody else's queue owns the answer.
var gating = flow.State{
	WhenIdle: flow.LetItRest, // the fleet's one gate slot owes the answer
	Name:     Gating,
	Title:    "Queued at the quality gate",
	About: "The worker's commit is waiting on the fleet's single gate slot. The gate's own result " +
		"moves it on — to a filed PR, or back to the failure it has to answer.",
	Says: says.Gating,
	Events: flow.Events{
		{cond.OwnPROpen, Submitted, "the gate passed and the pull request is out"},
		{cond.Rejected, Refreshing, "the gate failed it"},
		{cond.GateRefused, Working, "the gate answered and nothing landed — its output is the brief, and the work is still in hand"},
		{cond.TaskGone, Idle, "the work was closed under it"},
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.HoldsNothing, Idle, "it holds neither a task nor a feature, so it is not at work"},
	},
	Verbs: flow.Offers{{verb.Log, "record a note"}, {verb.Mail, "read your mailbox"}},
}

// submitted: a reviewer owes it a verdict.
var submitted = flow.State{
	WhenIdle: flow.LetItRest, // a reviewer owes the verdict
	Name:     Submitted,
	Title:    "Waiting on a verdict",
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
		{verb.Revoke, "withdraw the pull request"},
		{verb.Git, "read your changes"},
		{verb.Resolve, "check your branch still merges"},
		{verb.Rebase, "align onto the reference branch"},
		{verb.Task, "read the backlog"},
		{verb.Log, "record a note"},
		{verb.Mail, "read your mailbox"},
		{verb.Fyi, "one note to the user"},
		{verb.Escalate, "stop on a question"},
	},
}

// resolving: a conflict is in the agent's hands.
var resolving = flow.State{
	WhenIdle: flow.Nudge, // the conflict markers are in its workspace and only it can settle them
	Name:     Resolving,
	Title:    "Resolving a conflict",
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
		{verb.Resolve, "check the branch merges now"},
		{verb.Rebase, "align onto the reference branch"},
		{verb.Task, "read the backlog"},
		{verb.Mail, "read your mailbox"},
		{verb.Submit, "file it once it is clean"},
		{verb.Git, "read your changes"},
		{verb.Log, "record a note"},
		{verb.Escalate, "stop on a question"},
	},
}

// Flow is the worker's whole map, in the order a reader should meet it.
var Flow = []flow.State{
	idle, assigning, preparing, retiering, handingOver, working,
	refreshing, reworking, interviewing, submitting, gating, submitted, resolving,
	between, picking, featureGated, featureDone, releasing, yielding, promoting, rebasing,
	launching, stopping, clearing, escalated, retired, notDone, stalled,
}

// Start is where a worker with no state stored begins.
const Start = Idle
