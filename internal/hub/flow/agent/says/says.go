// package: hub/flow/agent/says / says
// type:    logic (what an agent is told where it stands, as identities)
// job:     name what each state answers when the agent asks where it is. The identity here, the
// words in the workflow — so a flow file declares what is said without carrying a paragraph of it.
// limits:  the names. The words are rendered against these, and a build check fails on one with none.
package says

// A state with no Says answers with its own action's outcome instead.
const (
	Escalated    = "escalated"     // the question it stopped on, repeated back
	Retired      = "retired"       // wound down by a human; nothing more is coming
	Coauthor     = "coauthor"      // it shares the user's seat; there is no queue
	Planner      = "planner"       // work reaches a planner as a conversation
	Planning     = "planning"      // carry on with the plan in hand
	Working      = "working"       // get on with the task in hand
	Rejected     = "rejected"      // the verdict came back, with the feedback to answer
	AwaitVerdict = "await-verdict" // its PR is with a reviewer
	Gating       = "gating"        // its commit is in the quality gate's queue
	Interviewing = "interviewing"  // a submit question stands against it, repeated back
	Resolving    = "resolving"     // a conflict is in its hands
	NoTasks      = "no-tasks"      // the backlog has nothing for it
	NoReviews    = "no-reviews"    // no PR is waiting on a verdict
	Reviewing    = "reviewing"     // the PR it holds, restated
	FeatureDone  = "feature-done"  // every subtask is closed; the feature can go up
	FeatureGated = "feature-gated" // what is left awaits the user's verdict
	Preparing    = "preparing"     // a running action is landing; the instruction follows it
	Stalled      = "stalled"       // it holds work and has stopped moving
	NotDone      = "not-done"      // it holds nothing and is not taking any, with something unanswered
)

// Refusals are the answers that tell an agent it has nothing to do here. Waking one standing in a
// state that says one of these hands it a refusal and nothing else — which is what the delivery
// path checks before every push (-> fleet.WakeRefusal).
// A running action is NOT one of them: news pushed at an agent mid-turn queues behind that turn and
// lands when it ends, so the hub being busy with it is no reason to say nothing.
var Refusals = map[string]string{
	Retired:   "wound down by the user — nothing will be assigned to it",
	Escalated: "stopped on a question only the user can answer",
}

// Refused is why waking an agent that is being told this would say nothing useful, "" when it can
// act on news.
func Refused(speech string) string { return Refusals[speech] }

// All is every declared speech, for the check that each has words and each is said by some state.
var All = []string{
	Escalated, Retired, Coauthor, Planner, Planning, Working, Rejected, AwaitVerdict,
	Gating, Interviewing, Resolving, NoTasks, NoReviews, Reviewing, FeatureDone, FeatureGated, Preparing,
	Stalled, NotDone,
}
