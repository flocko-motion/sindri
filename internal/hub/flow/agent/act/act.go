// package: hub/flow/agent/act / act
// type:    logic (the actions a state can run, as identities)
// job:     name every action a flow file may attach to a state, with the outcomes it can finish
// with — an identity and nothing more, so a flow file can name an action without importing anything
// that writes.
// limits:  the names and the outcome vocabulary. The implementations are registered against these
// (-> the acting halves under hub/flow), and a build check fails on a declared action with none.
package act

import "github.com/flo-at/sindri/internal/hub/flow"

// The outcomes actions finish with. Shared where they mean the same thing, so a state's Events list
// reads the same whichever action produced it.
var (
	Done    = flow.Outcome{Name: "done"}    // it worked
	Nothing = flow.Outcome{Name: "nothing"} // there was nothing to do
	Failed  = flow.Outcome{Name: "failed"}  // it did not work, and the reason is the agent's brief
	Queued  = flow.Outcome{Name: "queued"}  // handed to a queue; somebody else answers
	Stale   = flow.Outcome{Name: "stale"}   // what it was working from moved, so what it produced describes nothing
)

// PickWork looks for the best-rated unit the backlog would hand this agent, and claims it. It SAYS
// nothing: the hub selects, then prepares the session, and only then instructs (-> HandOver), so the
// claim holds the work while the preparation behind it runs and the agent hears once.
var PickWork = &flow.Action{Name: "pick-work", Outcomes: []flow.Outcome{Done, Nothing, Failed}}

// PickSubtask moves a feature holder onto its feature's next open child. It says nothing either, for
// the same reason: a child is selected, prepared for, and then handed over.
var PickSubtask = &flow.Action{Name: "pick-subtask", Outcomes: []flow.Outcome{Done, Nothing, Failed}}

// HandOver tells an agent what it holds. The LAST step of the chain, so nothing is ever said into a
// session still being prepared — and the only step the agent itself sees.
var HandOver = &flow.Action{Name: "hand-over", Outcomes: []flow.Outcome{Done, Failed}}

// Deliver serves an agent's unread mail INTO its session. The hub holds the messages and the agent
// is at an empty prompt, so waiting for it to come and fetch them waits for ever: reading is the
// only way out of not-done, and an idle agent has no reason to run anything.
var Deliver = &flow.Action{Name: "deliver", Outcomes: []flow.Outcome{Done, Nothing, Failed}}

// Clear discards the agent's session and waits for the reading to fall — the clear having HAPPENED,
// where a sleep only assumes it. This is the clear a HUMAN arms, and nothing runs behind it.
var Clear = &flow.Action{Name: "clear", Outcomes: []flow.Outcome{Done, Failed}}

// Prepare is the same reset as a step of a selection, and it is a separate action because a failure
// means something else here: the model switch behind it types `/model`, which on cached history
// opens a dialog that swallows whatever follows — the agent's own brief.
var Prepare = &flow.Action{Name: "prepare", Outcomes: []flow.Outcome{Done, Failed}}

// Retier switches the model under the agent for the tier of the work it is being handed. The switch
// clears the session on its way through.
var Retier = &flow.Action{Name: "retier", Outcomes: []flow.Outcome{Done, Failed}}

// Yield frees a container holder whose tree somebody else is working inside, and settles its PR.
var Yield = &flow.Action{Name: "yield", Outcomes: []flow.Outcome{Done}}

// Release drops a feature that has already landed and returns the agent to the backlog.
var Release = &flow.Action{Name: "release", Outcomes: []flow.Outcome{Done}}

// Interview puts the submit questions to an author and collects the answers, one exchange at a
// time. It WAITS on the agent — the whole sequence is one process, cancelled if anything moves the
// agent out from under it, which is what leaves a half-finished interview with nothing to reset.
var Interview = &flow.Action{Name: "interview", Outcomes: []flow.Outcome{Done, Stale, Failed}, Awaits: true}

// Submit files what the agent has for review, through the quality gate. Unattended: every question
// it might have asked has been answered by the time this runs.
var Submit = &flow.Action{Name: "submit", Outcomes: []flow.Outcome{Queued, Done, Failed}}

// TakeReview claims the oldest unclaimed review and puts its branch in the reviewer's workspace.
var TakeReview = &flow.Action{Name: "take-review", Outcomes: []flow.Outcome{Done, Nothing, Failed}}

// DropReview releases a review whose PR settled before a verdict, and tells the reviewer.
var DropReview = &flow.Action{Name: "drop-review", Outcomes: []flow.Outcome{Done}}

// Prod pushes an agent that holds work and has stopped doing it. It does not take the work away —
// a stall is an agent that needs waking, not one that has failed. Its own dedup keeps a long stall
// from being prodded on every beat.
var Prod = &flow.Action{Name: "prod", Outcomes: []flow.Outcome{Done, Nothing}}

// Promote turns the leaf task an agent holds into the feature it has become, so its children are
// worked through the one path that resumes an agent inside a feature rather than a second one
// beside it.
var Promote = &flow.Action{Name: "promote", Outcomes: []flow.Outcome{Done, Failed}}

// Rebase catches a standing branch up with a base its own milestone merge moved, KEEPING whatever
// the agent is mid-editing rather than discarding it.
var Rebase = &flow.Action{Name: "rebase", Outcomes: []flow.Outcome{Done, Failed}}

// Launch brings a reclaimed pod back up and waits for its session, so a claim is never made against
// an agent that cannot be told about it.
var Launch = &flow.Action{Name: "launch", Outcomes: []flow.Outcome{Done, Failed}}

// Stop takes an idle pod back, preserving the session so the cost of being wrong is the next start's
// latency and nothing else.
var Stop = &flow.Action{Name: "stop", Outcomes: []flow.Outcome{Done, Failed}}

// Disown releases work a role must not hold and puts it back in the backlog. A planner's work
// arrives as a conversation, so a backlog task on its row is a claim that should never have been
// made — and the planner's own map is what notices, wherever it stands.
var Disown = &flow.Action{Name: "disown", Outcomes: []flow.Outcome{Done, Failed}}

// All is every declared action, for the check that each has an implementation.
var All = []*flow.Action{
	PickWork, PickSubtask, Prepare, Retier, HandOver, Deliver, Clear, Yield, Release,
	Interview, Submit, TakeReview, DropReview, Prod, Promote, Rebase,
	Launch, Stop, Disown,
}
