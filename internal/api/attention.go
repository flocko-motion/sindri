// package: api / attention
// type:    logic (a rule over the board)
// job:     which agents and which PRs cannot move without the user — the rules behind
// every front-end's attention marker, written once so all of them count one set.
// limits:  pure rules over the board's own types; the section registry is hub-side.
package api

// Status words an agent wears while only the user can move it on.
const (
	StatusBlocked   = "blocked"    // stopped at a prompt, waiting for an answer
	StatusSignedOut = "signed-out" // the pane says to run /login, so nothing typed there is sent
	StatusStalled   = "stalled"    // holds work, screen standing still: it believes it is working
	// StatusEscalated: it asked the user to decide something and stopped on the answer. A word of its
	// own, not blocked: a runtime block is answered in the pane, this is answered and then resumed.
	StatusEscalated = "escalated"
	// StatusLaunchFailed: a launch was asked for and never came up (-> agent.Service.FailLaunch).
	// Distinct from "down": here somebody asked and it did not work.
	StatusLaunchFailed = "launch-failed"
	// StatusUnreachable: pod up, session up, and still nothing typed into it can be shown to have
	// arrived (-> agent.Service.Unreachable). Distinct from "down" and "signed-out": both of those are
	// read off the pane; this is what everything else looks like while that very reading may be stale.
	StatusUnreachable = "unreachable"
)

// AgentNeedsUser reports an agent whose state resolves ONLY IF A HUMAN ACTS — these six words today.
// Idle never counts, a full context included: an idle ask clears and reassigns itself. Retired is
// checked ahead of the rest since it reaches stalled by itself (-> workflow.parkedByTheHub).
func AgentNeedsUser(a AgentView) bool {
	// Ahead of retirement, unlike the rest: retirement reaches stalled by itself, but nothing about
	// it asks a question, and a retired agent still finishes what it holds.
	if a.Status == StatusEscalated {
		return true
	}
	if a.Retired {
		return false
	}
	switch a.Status {
	case StatusBlocked, StatusSignedOut, StatusStalled, StatusLaunchFailed, StatusUnreachable:
		return true
	}
	return false
}

// CountAgentsNeedingUser is how many of these agents are waiting on the user, off the field the hub
// decided rather than the rule again — a count that re-derived it could disagree with the markers.
func CountAgentsNeedingUser(agents []AgentView) (n int) {
	for _, a := range agents {
		if a.NeedsUser {
			n++
		}
	}
	return n
}

// PRWait is WHY a PR waits on the user — the classification itself, so a front-end maps it to
// words instead of deciding it a second time and disagreeing about a state added later.
type PRWait string

// The reasons, each a different thing to do about it.
const (
	PRWaitNone        PRWait = ""             // nothing is asked of the user
	PRWaitMergeFailed PRWait = "merge-failed" // a merge died in flight: base is in an unknown state
	PRWaitMerge       PRWait = "merge"        // approved, so it waits on the human-only merge
	PRWaitUserGated   PRWait = "user-gated"   // interim: no reviewer is ever asked for one
	PRWaitReview      PRWait = "review"       // open with no reviewer alive in its repo
)

// PRWaits is every reason, urgent first, so a caller groups over the set rather than a list of
// its own — and a reason added here reaches each of them.
var PRWaits = []PRWait{PRWaitMergeFailed, PRWaitMerge, PRWaitUserGated, PRWaitReview}

// PRWaitReason reports why nothing but a human will move this PR, PRWaitNone otherwise — merge-failed
// is most stuck, since no verb accepts it and nothing retries it. Interim is user-gated from the
// moment it opens: no reviewer is ever asked for one (-> workflow.needsReview).
func PRWaitReason(p PR, agents []AgentView) PRWait {
	switch {
	case p.Status == "merge-failed":
		return PRWaitMergeFailed
	case p.Status == "approved":
		return PRWaitMerge
	case p.Status != "open":
		return PRWaitNone
	case p.Kind == "interim":
		return PRWaitUserGated
	case !AnyLiveReviewer(agents, p.Project):
		return PRWaitReview
	}
	return PRWaitNone
}

// PRNeedsUser reports whether this PR waits on the user at all.
func PRNeedsUser(p PR, agents []AgentView) bool { return PRWaitReason(p, agents) != PRWaitNone }

// AnyLiveReviewer reports whether a reviewer agent is up AND assignable in that project. Excludes a
// retired one even though its status still reads up — it will never be assigned this review.
func AnyLiveReviewer(agents []AgentView, project string) bool {
	for _, a := range agents {
		if a.Project == project && a.Role == "reviewer" && !a.Retired && !AgentNotUp(a.Status) {
			return true
		}
	}
	return false
}

// CountPRsNeedingUser is how many of these PRs are waiting on the user.
func CountPRsNeedingUser(prs []PR, agents []AgentView) (n int) {
	for _, p := range prs {
		if PRNeedsUser(p, agents) {
			n++
		}
	}
	return n
}
