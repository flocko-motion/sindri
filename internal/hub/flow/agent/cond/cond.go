// package: hub/flow/agent/cond / cond
// type:    logic (the questions a state asks about the world)
// job:     every condition a flow file can watch, as a value it names — the question, the topics
// worth waking for, and nothing else. PURE: a condition is handed a gathered world and cannot
// query, write, or reach a clock.
// limits:  the questions. Where each one leads is the state's (-> flow/roles).
package cond

import (
	"time"

	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// of builds a condition: its name, how stale the answer may be, the topics that carry it early, and
// the question itself. The tolerance is declared HERE because it belongs to the question — the same
// wherever the question is asked — and every state watching it inherits its cadence from that.
func of(name string, within time.Duration, wake []flow.Topic, holds func(flow.World) bool) flow.Condition {
	return flow.Condition{Name: name, Within: within, Wake: wake, Holds: holds}
}

// --- what a human decided about the agent ---

// Retired: wound down by a human. Hands off every automatic behaviour.
var Retired = of("retired", flow.Soon, []flow.Topic{topic.AgentParked},
	func(w flow.World) bool { return w.Retired })

// BackInService: un-retired, so the ordinary flow resumes.
var BackInService = of("back-in-service", flow.Soon, []flow.Topic{topic.AgentParked},
	func(w flow.World) bool { return !w.Retired })

// ClearArmed: a human asked for this session to be discarded, AND there is something to discard.
// Both halves matter. A clear typed into an empty session never lands — the harness waits for a
// reading to FALL and one that was never taken cannot — so entering the clearing state over it
// would leave and re-enter for ever, four rows on the agent's record per beat. The arming stays
// until there is context to answer it with, which is the moment it means anything.
var ClearArmed = of("clear-armed", flow.Soon, []flow.Topic{topic.AgentParked},
	func(w flow.World) bool { return w.ClearArmed && w.Up && w.Fill > 0 })

// Escalated: the agent stopped on a question only the user can answer.
var Escalated = of("escalated", flow.Soon, nil, func(w flow.World) bool { return w.Escalation != "" })

// Resolved: the escalation was answered and cleared.
var Resolved = of("resolved", flow.Blocking, nil, func(w flow.World) bool { return w.Escalation == "" })

// --- the work in hand ---

// TaskGone: the work it holds is no longer in the backlog — closed, scrapped, or given away.
var TaskGone = of("task-gone", flow.Soon, []flow.Topic{topic.TaskClosed, topic.HoldingChanged},
	func(w flow.World) bool { return w.TaskGone })

// WorkAvailable: the backlog holds something rated for this agent, AND its pod is up to be told. A
// claim against a dead pod erases the row saying work was waiting, and nothing brings it back for it.
var WorkAvailable = of("work-available", flow.Soon, []flow.Topic{topic.TaskAvailable, topic.TaskApproved},
	func(w flow.World) bool { return w.HasNext && w.Up })

// SessionInTheWay: work waits and the session still holds the last unit. Work arrives WHOLE, so that
// unit is context to drop. Fill > 0 because a clear typed into an empty session never lands.
var SessionInTheWay = of("session-in-the-way", flow.Soon, []flow.Topic{topic.TaskAvailable, topic.SessionRead},
	func(w flow.World) bool { return w.Up && w.Fill > 0 && (w.HasNext || len(w.Subtasks) > 0) })

// TierMismatch: the work waiting is rated for another model, so the switch comes before the
// hand-over — it restarts the session on its way through.
var TierMismatch = of("tier-mismatch", flow.Soon, []flow.Topic{topic.TaskAvailable},
	func(w flow.World) bool { return w.TierMismatch && w.Up && (w.HasNext || len(w.Subtasks) > 0) })

// BetweenSubtasks: it holds a feature and no child of it — a LEAF BOUNDARY, whatever it was doing
// before. An agent left "working" with nothing in hand is one the board shows as busy and the
// subtask loop never reaches.
var BetweenSubtasks = of("between-subtasks", flow.Soon, []flow.Topic{topic.TaskClosed, topic.PRMerged, topic.HoldingChanged},
	func(w flow.World) bool { return w.Container != "" && w.Task == "" })

// HoldsBacklogWork: a backlog task or feature is on this agent's row — an invalid claim for a role
// whose work does not come from the backlog, noticed by the agent holding it.
var HoldsBacklogWork = of("holds-backlog-work", flow.Soon, []flow.Topic{topic.TaskAvailable, topic.HoldingChanged},
	func(w flow.World) bool { return w.Task != "" || w.Container != "" })

// HoldsFeature: it holds a container task, so it is inside the subtask loop rather than a leaf one.
var HoldsFeature = of("holds-feature", flow.Blocking, []flow.Topic{topic.HoldingChanged},
	func(w flow.World) bool { return w.Container != "" })

// --- a feature and its tree ---

// FeatureGone: the feature it held is closed at its source, or landed without it.
var FeatureGone = of("feature-gone", flow.Soon, []flow.Topic{topic.TaskClosed, topic.PRMerged},
	func(w flow.World) bool { return w.Container != "" && w.FeatureLanded })

// TreeSplit: another agent is working inside the tree this one holds the container of. The container
// holder yields — the leaf is the concrete work.
var TreeSplit = of("tree-split", flow.Soon, []flow.Topic{topic.TaskAvailable},
	func(w flow.World) bool { return w.SplitTree })

// SubtaskReady: the held feature has an open child to hand over.
var SubtaskReady = of("subtask-ready", flow.Blocking, []flow.Topic{topic.TaskApproved, topic.TaskAvailable, topic.HoldingChanged},
	func(w flow.World) bool { return len(w.Subtasks) > 0 })

// SubtasksGated: nothing is claimable because what is left awaits the user's verdict.
var SubtasksGated = of("subtasks-gated", flow.Soon, []flow.Topic{topic.TaskApproved},
	func(w flow.World) bool { return len(w.Subtasks) == 0 && len(w.Gated) > 0 })

// FeatureFinished: every child is closed, so the feature itself can go up.
var FeatureFinished = of("feature-finished", flow.Soon, []flow.Topic{topic.TaskClosed, topic.HoldingChanged},
	func(w flow.World) bool { return len(w.Subtasks) == 0 && len(w.Gated) == 0 })

// --- verdicts ---

// NotRejected: NO verdict stands against this agent — neither on the work in hand nor on a pull
// request it has out. Both, because the round it is answering can be filed against either.
var NotRejected = of("not-rejected", flow.Soon, []flow.Topic{topic.PRVerdict},
	func(w flow.World) bool { return !w.Held.Rejected && !w.Awaiting.Rejected })

// Rejected: the verdict on the work in hand came back asking for another round.
var Rejected = of("rejected", flow.Soon, []flow.Topic{topic.PRVerdict},
	func(w flow.World) bool { return w.Held.Rejected })

// OwnPRRejected: a PR it filed came back rejected while it holds nothing else.
var OwnPRRejected = of("own-pr-rejected", flow.Soon, []flow.Topic{topic.PRVerdict},
	func(w flow.World) bool { return w.Awaiting.Rejected })

// OwnPROpen: a PR of its own is still to land, so it is answerable for that before anything new.
var OwnPROpen = of("own-pr-open", flow.Soon, []flow.Topic{topic.PRVerdict, topic.PRMerged},
	func(w flow.World) bool { return w.AwaitingPR != "" && !w.Awaiting.Rejected })

// AwaitingContribution: an interim pull request of its own is out, and the user has to land it
// before the branch it would carry on can move. A LEAF only — a feature worker keeps its subtasks
// going, since a milestone is an instalment of a branch it has not finished.
var AwaitingContribution = of("awaiting-contribution", flow.Soon, []flow.Topic{topic.PRVerdict, topic.PRMerged},
	func(w flow.World) bool { return w.Container == "" && w.AwaitingPR != "" && !w.Awaiting.Rejected })

// PRSettled: the PR it was waiting on is no longer open — merged, scrapped or withdrawn.
var PRSettled = of("pr-settled", flow.Soon, []flow.Topic{topic.PRMerged, topic.PRVerdict},
	func(w flow.World) bool { return w.AwaitingPR == "" })

// MergeConflicted: the merge of a PR this agent filed hit a conflict and handed the branch back.
// Read off the PR's own record rather than written onto the agent by whoever merged: a merge writes
// its own subject, and what a conflict MEANS for the author is decided here.
var MergeConflicted = of("merge-conflicted", flow.Blocking, []flow.Topic{topic.PRVerdict},
	func(w flow.World) bool { return w.Conflicted })

// GainedChildren: the leaf task this agent holds has grown children, so its unit of work is a
// FEATURE now. It takes the new work on rather than being stranded in front of it or merging over it.
var GainedChildren = of("gained-children", flow.Soon, []flow.Topic{topic.TaskAvailable, topic.TaskApproved},
	func(w flow.World) bool { return w.GainedChildren })

// MilestoneLanded: an interim pull request of its own merged while it still holds the work. Its
// standing branch is behind the base that merge moved, and the uncommitted work on it must survive
// catching up.
var MilestoneLanded = of("milestone-landed", flow.Blocking, []flow.Topic{topic.PRMerged},
	func(w flow.World) bool { return w.MilestoneLanded })

// NoConflict: nothing stands unresolved against a pull request this agent filed. Leaving the
// resolving state is the ABSENCE of the conflict rather than the resolve verb succeeding: an agent
// that fixed the branch by hand is as resolved as one that ran the verb.
var NoConflict = of("no-conflict", flow.Blocking, []flow.Topic{topic.PRVerdict, topic.PRMerged},
	func(w flow.World) bool { return !w.Conflicted })

// --- a submit ---

// SubmitAsked: the author asked for a submit and it has not been taken. The questions come before
// anything is committed, so this leads to the interview rather than to the gate.
var SubmitAsked = of("submit-asked", flow.Soon, []flow.Topic{topic.SubmitAsked},
	func(w flow.World) bool { return w.SubmitAsked })

// NoSubmitAsked: the submit an author was answering for is gone — carried out, or called off. A
// repair: an interview outliving its request would stand for ever, since nothing else names it.
var NoSubmitAsked = of("no-submit-asked", flow.Soon, []flow.Topic{topic.SubmitAsked},
	func(w flow.World) bool { return !w.SubmitAsked })

// GateRefused: the quality gate it was standing at answered, and nothing landed. Its output is the
// brief, delivered as a message — so the work is still in hand and there is nothing to re-read.
var GateRefused = of("gate-refused", flow.Soon, []flow.Topic{topic.GateFinished},
	func(w flow.World) bool { return w.GateRefused })

// --- a reviewer's PR ---

// ReviewHeld: it holds one pull request whose branch is in its workspace. A reviewer handed one by
// any route arrives here, which is what stops a hand-over having to write where it stands.
var ReviewHeld = of("review-held", flow.Blocking, []flow.Topic{topic.PRVerdict},
	func(w flow.World) bool { return w.ReviewingPR != "" && w.ReviewOpen })

// ReviewOvertaken: the PR it was reading settled before it gave a verdict, so the hold means nothing.
var ReviewOvertaken = of("review-overtaken", flow.Soon, []flow.Topic{topic.PRMerged, topic.PRVerdict},
	func(w flow.World) bool { return w.ReviewingPR != "" && !w.ReviewOpen })

// ReviewDone: it holds no review any more, having ruled on the one it had. The verdict RECORDED is
// what releases a reviewer — a verdict is a fact about the pull request, and nothing writes a
// release beside it, which is how two writers of where a reviewer stands came to disagree.
var ReviewDone = of("review-done", flow.Soon, []flow.Topic{topic.PRVerdict},
	func(w flow.World) bool { return w.ReviewingPR == "" })

// ReviewWaiting: a pull request is filed with nobody reading it, and this reviewer may take it. One
// workspace, so one PR. A dead pod cannot be told about a claim — which is how a filed review went
// unread by every reviewer at once. A review is read on a CLEARED session, and clearing one mid-turn
// discards that turn. Whether it may be handed anything at all is the surface's, asked not restated.
var ReviewWaiting = of("review-waiting", flow.Soon, []flow.Topic{topic.ReviewFiled, topic.PRVerdict},
	func(w flow.World) bool {
		return w.ReviewingPR == "" && w.ReviewWaiting && w.Up && w.AtPrompt() && w.Allowed().Assign == ""
	})

// --- mail, and standing still ---

// MailWaiting: the agent has messages it has not read. Unread mail means it is not DONE, and an
// agent that is not done is not prepared for new work — which is what stops a clear landing on top
// of something nobody has seen.
// Up as well: a stopped agent reads nothing, and the mail state would hold it past its own launch.
var MailWaiting = of("mail-waiting", flow.Soon, []flow.Topic{topic.MailArrived},
	func(w flow.World) bool { return w.Unread > 0 && w.Up })

// MailRead: nothing is waiting in the mailbox.
var MailRead = of("mail-read", flow.Soon, []flow.Topic{topic.MailArrived},
	func(w flow.World) bool { return w.Unread == 0 })

// Stalled: the agent holds work and its screen has stopped changing. A pane frozen mid-turn keeps
// SAYING "working" for ever, so stillness rather than the word is the evidence.
var Stalled = of("stalled", flow.Soon, []flow.Topic{topic.SessionRead},
	func(w flow.World) bool { return w.ScreenStalled() })

// Moving: the screen is changing again, so whatever the stall was, it is over.
var Moving = of("moving", flow.Soon, []flow.Topic{topic.SessionRead},
	func(w flow.World) bool { return !w.ScreenStalled() })

// --- a planner's conversation ---

// InConversation: something has been said into this agent's session. A planner's work IS the
// conversation, which is why it declares nothing about where it stands — the session is the fact.
var InConversation = of("in-conversation", flow.Soon, []flow.Topic{topic.SessionRead},
	func(w flow.World) bool { return w.Up && w.Fill > 0 })

// ConversationOver: the session was READ and holds nothing. Both halves — an unmeasured session
// reports a zero fill too, and every planner nobody has looked at would leave the plan it is inside.
var ConversationOver = of("conversation-over", flow.Soon, []flow.Topic{topic.SessionRead},
	func(w flow.World) bool { return w.Window > 0 && w.Fill == 0 })

// --- the session ---

// SessionGone: the pod is not running, so nothing can be handed over yet.
var SessionGone = of("session-gone", flow.Eventually, []flow.Topic{topic.SessionRead},
	func(w flow.World) bool { return !w.Up })

// SessionUp: the pod is running, so a launch has arrived wherever it came from.
var SessionUp = of("session-up", flow.Eventually, []flow.Topic{topic.SessionRead},
	func(w flow.World) bool { return w.Up })

// --- the pod ---

// NeededWhileAsleep: its OWN kind of work waits, its pod was reclaimed, nobody awake would take it,
// and of those asleep this is the first. Waking is the missing half of reclaiming; one wake, one pod.
var NeededWhileAsleep = of("needed-while-asleep", flow.Soon,
	[]flow.Topic{topic.TaskAvailable, topic.TaskApproved, topic.PRVerdict},
	func(w flow.World) bool {
		return w.Wanted && w.Stopped && !w.Up && !w.PoolCovered && w.FirstAsleep &&
			w.Allowed().Wake == ""
	})

// AsleepHolding: it holds work and has no pod to do it in. A claim must not outlive its pod, and
// retirement — the way a human says "no more work" — empties the agent first.
var AsleepHolding = of("asleep-holding", flow.Soon, []flow.Topic{topic.SessionRead},
	func(w flow.World) bool { return w.Stopped && !w.Up && !w.HoldsNothing() })

// StartAsked: a human asked for this pod. A request, since the agent may already be in whatever
// state a flag would claim.
var StartAsked = of("start-asked", flow.Soon, []flow.Topic{topic.AgentParked},
	func(w flow.World) bool { return w.StartAsked })

// StopAsked: a human asked for this pod back, whatever it is holding. Their call, not the sweep's.
var StopAsked = of("stop-asked", flow.Soon, []flow.Topic{topic.AgentParked},
	func(w flow.World) bool { return w.StopAsked })

// Reclaimable: it has held nothing long enough that its pod is worth taking back. WHETHER it may be
// is the surface's rule, read not restated — which is what exempts a coauthor's seat.
var Reclaimable = of("reclaimable", flow.Eventually, nil,
	func(w flow.World) bool {
		return w.Allowed().Reclaim == "" && w.Up && w.InPhase >= flow.IdleStopThreshold && !w.Wanted
	})

// LaunchOverdue: a launch has stood longer than one takes. The bound belongs to the state, not to a
// sweep watching from outside it.
var LaunchOverdue = of("launch-overdue", flow.Soon, nil,
	func(w flow.World) bool { return w.InPhase >= flow.LaunchBound })

// All is every declared condition, for the check that each one is watched by some state.
var All = []flow.Condition{
	Retired, BackInService, ClearArmed, Escalated, Resolved,
	TaskGone, WorkAvailable,
	HoldsFeature, HoldsBacklogWork, SessionInTheWay, TierMismatch, BetweenSubtasks, FeatureGone, TreeSplit, SubtaskReady, SubtasksGated, FeatureFinished,
	NotRejected, Rejected, OwnPRRejected, OwnPROpen, AwaitingContribution, PRSettled, MergeConflicted,
	GainedChildren, MilestoneLanded, NoConflict,
	SubmitAsked, NoSubmitAsked, GateRefused, ReviewHeld, ReviewOvertaken, ReviewDone, ReviewWaiting,
	MailWaiting, MailRead, Stalled, Moving,
	SessionGone, SessionUp, InConversation, ConversationOver,
	NeededWhileAsleep, AsleepHolding, StartAsked, StopAsked, Reclaimable, LaunchOverdue,
}
