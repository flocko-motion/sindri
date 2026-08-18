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
	StatusFull      = "full"       // past its context window holding nothing: no work until cleared
	StatusStalled   = "stalled"    // holds work, screen standing still: it believes it is working
	// StatusEscalated: it asked the user to decide something and stopped on the answer. A word of its
	// own, not blocked: a runtime block is answered in the pane, this is answered and then resumed.
	StatusEscalated = "escalated"
)

// AgentNeedsUser reports an agent whose state resolves ONLY IF A HUMAN ACTS. That is the rule, and
// these five words satisfy it today; a status added later is asked the same question. Idle never
// counts: waiting for work is normal, and idling beside claimable work is the hub's to nudge. An
// api-error is the hub's to resend, and once resending fails the board says stalled. Retired is
// checked ahead of the status because it REACHES the counting states — a retired agent keeps running,
// so it fills up or stalls, and a marker would never clear (-> workflow.parkedByTheHub, same rule).
func AgentNeedsUser(a AgentView) bool {
	// Ahead of retirement, unlike the rest: retirement reaches full and stalled by itself, but nothing
	// about it asks a question, and a retired agent still finishes what it holds.
	if a.Status == StatusEscalated {
		return true
	}
	if a.Retired {
		return false
	}
	switch a.Status {
	case StatusBlocked, StatusSignedOut, StatusFull, StatusStalled:
		return true
	}
	return false
}

// CountAgentsNeedingUser is how many of these agents are waiting on the user.
func CountAgentsNeedingUser(agents []AgentView) (n int) {
	for _, a := range agents {
		if AgentNeedsUser(a) {
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

// PRWaitReason reports why nothing but a human will move this PR, PRWaitNone when something else
// will. Merge-failed is most stuck: a restart caught the merge in flight (-> ReconcileMergingPRs),
// nothing retries it and no verb accepts it, so only a person reading the base branch moves it on.
// Approved waits on the merge, the one hard gate. Interim is user-gated from the moment it opens —
// no reviewer is ever asked for one (-> workflow.needsReview). Open with no reviewer alive IN ITS
// OWN REPO waits on a review that is not coming, though one merely unassigned is ordinary while a
// reviewer runs there. A rejected PR waits on its author, a merging one on the merge under way.
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

// AnyLiveReviewer reports whether a reviewer agent is up in that project. Scoped because
// assignment is (-> workflow.freeReviewer reads one project's roster), so a reviewer up in another
// repo will never be handed this PR.
func AnyLiveReviewer(agents []AgentView, project string) bool {
	for _, a := range agents {
		if a.Project == project && a.Role == "reviewer" && !AgentNotUp(a.Status) {
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
