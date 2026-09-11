// package: hub/flow/pr / pr
// type:    logic (a pull request's own flow, declared)
// job:     the map of a merge intent's life — filed, gated, read, ruled on, merged or discarded —
// with the conditions that move it. A PR writes only ITS OWN subject: what a merge means for the
// agent that filed it, or the task it answers, is decided by their maps, not by this one.
// limits:  the map and the world it reads. Merging, scrapping and requesting a review are the
// workflow's (-> flowpr.go).
package pr

import (
	"time"

	"github.com/flo-at/sindri/internal/hub/flow/machine"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// The states a merge intent can stand in. Each is the status it claims, except merging, which is a
// running action and says so.
const (
	Filed      = "pr/filed"
	Requesting = "pr/requesting-review"
	Gating     = "pr/gating"
	Reviewing  = "pr/reviewing"
	Approved   = "pr/approved"
	Rejected   = "pr/rejected"
	Merging    = "pr/merging"
	Merged     = "pr/merged"
	Scrapped   = "pr/scrapped"
	Stuck      = "pr/stuck"
)

// The engine's shapes bound to a merge intent's world, so this file names them without a type
// parameter in sight.
type (
	State     = machine.State[World]
	Condition = machine.Condition[World]
	Events    = []machine.Transition[World]
	Outcome   = machine.Outcome
)

// The outcomes a merge intent's actions finish with, and the two actions themselves. Named here
// beside the states that run them, because a PR has few enough of both to read as one page.
var (
	Opened     = Outcome{Name: "opened"}     // a review row was opened
	Nothing    = Outcome{Name: "nothing"}    // there was nothing to do
	Landed     = Outcome{Name: "landed"}     // the base carries it
	Conflicted = Outcome{Name: "conflicted"} // the rebase conflicts; the branch is back with its author
	Refused    = Outcome{Name: "refused"}    // the merge could not run at all, and nothing moved

	// AskForReview opens the review row a filed pull request needs to be claimable. It is reached by
	// the ABSENCE of a row, which is what makes it the repair as well as the ordinary path: a pull
	// request whose row was lost stands unreviewed, and there is no second sweep to notice that.
	AskForReview = &machine.Action{Name: "ask-for-review", Outcomes: []Outcome{Opened, Nothing}}

	// DoMerge rebases the branch onto its base and commits it there. It is an ACTION rather than a
	// moment inside a verb because a merge is not atomic: a hub that died half way through leaves
	// nobody knowing whether the base carries it, which is what pr/stuck is for.
	DoMerge = &machine.Action{Name: "merge", Outcomes: []Outcome{Landed, Conflicted, Refused}}
)

// World is everything a PR's conditions read, gathered once per pass.
type World struct {
	ID      string
	Project string
	Exists  bool
	// Task is what it answers, and TaskOpen whether that task is still worth answering. A PR against
	// a closed task can never land, and leaving it open made every reader carry the exception.
	Task     string
	TaskOpen bool
	// Interim: a mid-task contribution. Its merge is a milestone, not the task's end.
	Interim bool
	// Reviewer holds it, "" when nobody does. ReviewOpen: that review is still owed a verdict.
	Reviewer   string
	ReviewOpen bool
	// Verdict is what came back: "approved", "rejected", or "" for nothing yet.
	Verdict string
	// GatePassed: the quality gate ruled on this commit. GatePending: it is queued behind the fleet's
	// single slot.
	GatePassed  bool
	GatePending bool
	// ReviewFiled: a live review row covers this, claimed or not. Distinct from Reviewer, which is
	// somebody HOLDING it: a filed PR with no row at all is one no reviewer can ever be handed.
	ReviewFiled bool
	// MergeAsked: a human asked for this merge and it has not been carried out.
	MergeAsked bool
	// Conflicted: a conflict is recorded against this pull request, so the branch went back to its
	// author. Read off the record rather than returned by a call, so a merge whose result was lost
	// still settles.
	Conflicted bool
	// Landed: the branch is on its base. BaseMoved: the base has advanced since it was filed.
	Landed    bool
	BaseMoved bool
}

func of(name string, within time.Duration, wake []machine.Topic, holds func(World) bool) Condition {
	return Condition{Name: name, Within: within, Wake: wake, Holds: holds}
}

// How stale an answer about a PR may be. A PR blocks the agent that filed it, so its questions are
// asked more attentively than a task's.
const (
	prompt = 15 * time.Second
	soon   = time.Minute
)

var (
	taskGone   = of("task-gone", soon, []machine.Topic{topic.TaskClosed}, func(w World) bool { return !w.TaskOpen })
	vanished   = of("vanished", soon, []machine.Topic{topic.PRMerged}, func(w World) bool { return !w.Exists })
	gateQueued = of("gate-queued", prompt, []machine.Topic{topic.GateFinished}, func(w World) bool { return w.GatePending })
	gatePassed = of("gate-passed", prompt, []machine.Topic{topic.GateFinished},
		func(w World) bool { return w.GatePassed && !w.GatePending })
	held     = of("reviewer-holds-it", soon, []machine.Topic{topic.PRVerdict}, func(w World) bool { return w.Reviewer != "" && w.ReviewOpen })
	unheld   = of("nobody-holds-it", soon, []machine.Topic{topic.PRVerdict}, func(w World) bool { return w.Reviewer == "" || !w.ReviewOpen })
	ruledOK  = of("approved", prompt, []machine.Topic{topic.PRVerdict}, func(w World) bool { return w.Verdict == "approved" })
	ruledNo  = of("rejected", prompt, []machine.Topic{topic.PRVerdict}, func(w World) bool { return w.Verdict == "rejected" })
	reopened = of("verdict-withdrawn", soon, []machine.Topic{topic.PRVerdict}, func(w World) bool { return w.Verdict == "" })
	landed   = of("landed", prompt, []machine.Topic{topic.PRMerged}, func(w World) bool { return w.Landed })
	// unreviewed: filed, wanting a review, and no live row covering it. An interim PR wants none —
	// a mid-task contribution and a milestone both wait on the human.
	unreviewed = of("unreviewed", soon, []machine.Topic{topic.PRVerdict},
		func(w World) bool { return w.Exists && !w.Interim && !w.ReviewFiled })
	mergeAsked = of("merge-asked", prompt, []machine.Topic{topic.PRVerdict}, func(w World) bool { return w.MergeAsked })
	conflicted = of("conflicted", prompt, []machine.Topic{topic.PRMerged}, func(w World) bool { return w.Conflicted })
)

// filed: a merge intent exists and needs somebody to look at it.
var filed = State{
	Name:  Filed,
	Title: "Filed, nobody reading it yet",
	About: "The branch is recorded as a merge intent, a live review row covers it, and no reviewer " +
		"holds it yet. A reviewer with one workspace can hold exactly one pull request, so this waits " +
		"for one to come free rather than being pushed at somebody mid-review.",
	Events: Events{
		{vanished, Scrapped, "the record is gone"},
		{taskGone, Scrapped, "the task it answers closed, so it can never land"},
		{gateQueued, Gating, "the quality gate took its commit"},
		{held, Reviewing, "a reviewer took it"},
		{ruledOK, Approved, "a verdict came back approving it"},
		{ruledNo, Rejected, "a verdict came back rejecting it"},
		{unreviewed, Requesting, "no live row covers it, so no reviewer can ever be handed it"},
	},
}

// requesting: the row that makes a filed pull request claimable is being opened.
var requesting = State{
	Name:   Requesting,
	Title:  "Opening its review row",
	Action: AskForReview,
	About: "Nothing covers this pull request, so no reviewer can be handed it. The row is opened " +
		"here — both when it is first filed and whenever one is later lost, since a pull request " +
		"without one stands in exactly this state either way and no sweep is left to find it.",
	Events: Events{
		{Opened, Filed, "the row is open; it waits for a reviewer to come free"},
		{Nothing, Filed, "a live row already covers it"},
		{vanished, Scrapped, "the record is gone"},
		{taskGone, Scrapped, "the task it answers closed"},
		{held, Reviewing, "a reviewer took it while the row was being opened"},
		{machine.Orphaned{}, Filed, "the hub restarted mid-request"},
	},
}

// gating: the fleet's one gate slot has its commit.
var gating = State{
	Name:  Gating,
	Title: "At the quality gate",
	About: "Its commit is queued behind the fleet's single gate slot. The gate's own result decides: " +
		"a pass puts it in front of a reviewer, a failure sends it back to its author.",
	Events: Events{
		{vanished, Scrapped, "the record is gone"},
		{taskGone, Scrapped, "the task it answers closed"},
		{gatePassed, Filed, "the gate passed it, so it is ready for a reviewer"},
		{ruledNo, Rejected, "the gate failed it"},
	},
}

// reviewing: one reviewer has it checked out.
var reviewing = State{
	Name:  Reviewing,
	Title: "Being read",
	About: "A reviewer holds this and its branch is in that reviewer's workspace. A PR that settles " +
		"under a reviewer releases the hold rather than waiting for a verdict that would decide " +
		"nothing.",
	Events: Events{
		{vanished, Scrapped, "the record is gone"},
		{taskGone, Scrapped, "the task it answers closed while it was being read"},
		{ruledOK, Approved, "the reviewer approved it"},
		{ruledNo, Rejected, "the reviewer sent it back"},
		{unheld, Filed, "the reviewer let go of it without ruling"},
	},
}

// approved: waiting on a human. The single hard gate in the whole system.
var approved = State{
	Name:  Approved,
	Title: "Approved, waiting on a human",
	About: "A reviewer approved this and nothing else in sindri may merge it. That is the one hard " +
		"gate in the system and it has no action here on purpose: a human types `merge`, and only a " +
		"human ever does.",
	Events: Events{
		{vanished, Scrapped, "the record is gone"},
		{taskGone, Scrapped, "the task it answers closed before anyone merged it"},
		{landed, Merged, "it went in"},
		{mergeAsked, Merging, "a human asked for the merge — the one hard gate, answered"},
		{reopened, Filed, "the approval was withdrawn, so it needs reading again"},
		{ruledNo, Rejected, "the approval was replaced by a rejection"},
	},
}

// rejected: back with its author, who owns the next round.
var rejected = State{
	Name:  Rejected,
	Title: "Sent back to its author",
	About: "A verdict came back asking for another round. The author's own flow is what acts on that; " +
		"this record simply waits to be superseded, withdrawn, or picked up again.",
	Events: Events{
		{vanished, Scrapped, "the record is gone"},
		{taskGone, Scrapped, "the task it answers closed"},
		{reopened, Filed, "the rejection was withdrawn"},
		{ruledOK, Approved, "it was approved after all"},
	},
}

// merging: the merge is running, and a merge is not atomic.
var merging = State{
	Name:   Merging,
	Title:  "Merging",
	Action: DoMerge,
	About: "The merge is running: rebase, then the commit onto the base. A hub that died mid-merge " +
		"leaves nobody knowing whether the base carries it, which is why this state exists rather " +
		"than the merge being a moment inside a verb. It is left by OBSERVATION as well as by " +
		"outcome, so a result that was lost still settles: a base already carrying the branch is " +
		"merged, and a recorded conflict is a conflict, whatever the action returned.",
	Events: Events{
		{Landed, Merged, "it went in"},
		{Conflicted, Filed, "the rebase conflicts; the branch is back with its author to resolve"},
		{Refused, Approved, "the merge could not run — it stays approved and retryable"},
		{landed, Merged, "the base carries it"},
		{conflicted, Filed, "a conflict is recorded against it"},
		{machine.Orphaned{}, Stuck, "the hub restarted mid-merge — whether the base carries it is unknown"},
	},
}

// stuck: a merge whose outcome nobody knows.
var stuck = State{
	Name:  Stuck,
	Title: "Merge outcome unknown",
	About: "A hub died while this was merging, so half of it may be on the base. This asks for a " +
		"human to look — `merging` would not, and a status that quietly reads in-flight for ever is " +
		"how a half-merge goes unnoticed. Only their own fresh approval moves it: a withdrawn verdict " +
		"would leave it here too, and reading that as permission to try again is the opposite of " +
		"what this state is for.",
	Events: Events{
		{landed, Merged, "a look confirmed the base carries it"},
		{ruledOK, Approved, "a human approved it again, for another attempt"},
	},
}

// merged and scrapped are the two ends. Both are terminal, and both are still watched: a task that
// reopens beneath a merged PR is somebody else's problem, not this record's.
var merged = State{
	Name:  Merged,
	Title: "Merged",
	About: "It went in. Everything that follows — the task closing, the agent being released, the " +
		"planners rebasing — belongs to those subjects' own maps, woken by this one landing.",
	Events: Events{{vanished, Scrapped, "the record is gone"}},
}

var scrapped = State{
	Name:  Scrapped,
	Title: "Scrapped",
	About: "Discarded without landing. The branch is untouched: what is thrown away is the intent " +
		"to merge it, not the work.",
	Events: Events{{vanished, Scrapped, "the record is gone"}},
}

// Flow is a pull request's whole map.
var Flow = []State{filed, requesting, gating, reviewing, approved, rejected, merging, stuck, merged, scrapped}

// Start is where a merge intent begins.
const Start = Filed

// Conditions is every declared condition, for the check that each is watched.
var Conditions = []Condition{taskGone, vanished, gateQueued, gatePassed, held, unheld, ruledOK, ruledNo,
	reopened, landed, unreviewed, mergeAsked, conflicted}
