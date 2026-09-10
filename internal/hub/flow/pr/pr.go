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
	Filed     = "pr/filed"
	Gating    = "pr/gating"
	Reviewing = "pr/reviewing"
	Approved  = "pr/approved"
	Rejected  = "pr/rejected"
	Merging   = "pr/merging"
	Merged    = "pr/merged"
	Scrapped  = "pr/scrapped"
	Stuck     = "pr/stuck"
)

// The engine's shapes bound to a merge intent's world, so this file names them without a type
// parameter in sight.
type (
	State     = machine.State[World]
	Condition = machine.Condition[World]
	Events    = []machine.Transition[World]
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
)

// filed: a merge intent exists and needs somebody to look at it.
var filed = State{
	Name:  Filed,
	Title: "Filed, nobody reading it yet",
	About: "The branch is recorded as a merge intent and no reviewer holds it. A reviewer with one " +
		"workspace can hold exactly one pull request, so this waits for one to come free rather " +
		"than being pushed at somebody mid-review.",
	Events: Events{
		{vanished, Scrapped, "the record is gone"},
		{taskGone, Scrapped, "the task it answers closed, so it can never land"},
		{gateQueued, Gating, "the quality gate took its commit"},
		{held, Reviewing, "a reviewer took it"},
		{ruledOK, Approved, "a verdict came back approving it"},
		{ruledNo, Rejected, "a verdict came back rejecting it"},
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
	Name:  Merging,
	Title: "Merging",
	About: "The merge is running: rebase, then the commit onto the base. A hub that died mid-merge " +
		"leaves nobody knowing whether the base carries it, which is why this state exists rather " +
		"than the merge being a moment inside a verb.",
	Events: Events{
		{landed, Merged, "the base carries it"},
		{machine.Orphaned{}, Stuck, "the hub restarted mid-merge — whether the base carries it is unknown"},
	},
}

// stuck: a merge whose outcome nobody knows.
var stuck = State{
	Name:  Stuck,
	Title: "Merge outcome unknown",
	About: "A hub died while this was merging, so half of it may be on the base. This asks for a " +
		"human to look — `merging` would not, and a status that quietly reads in-flight for ever is " +
		"how a half-merge goes unnoticed.",
	Events: Events{
		{landed, Merged, "a look confirmed the base carries it"},
		{reopened, Filed, "it was re-approved for another attempt"},
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
var Flow = []State{filed, gating, reviewing, approved, rejected, merging, stuck, merged, scrapped}

// Start is where a merge intent begins.
const Start = Filed

// Conditions is every declared condition, for the check that each is watched.
var Conditions = []Condition{taskGone, vanished, gateQueued, gatePassed, held, unheld, ruledOK, ruledNo, reopened, landed}
