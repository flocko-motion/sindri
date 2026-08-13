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
)

// AgentNeedsUser reports an agent whose state resolves ONLY IF A HUMAN ACTS. That is the rule, and
// these four words are what satisfies it today; a status added later is asked the same question.
//
// Idle never counts: waiting for work is normal, and idling beside claimable work is the hub's to
// nudge. An api-error is the hub's to resend, and once resending fails the board says stalled.
// Retired is checked ahead of the status because it REACHES the counting states — a retired agent
// keeps running, so it fills up or stalls, and a marker would sit on it until it was deleted (the
// stall nudge exempts it likewise: -> workflow.parkedByTheHub).
func AgentNeedsUser(a AgentView) bool {
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

// PRNeedsUser reports a PR nothing but a human will move: approved (it waits on the merge, the one
// hard gate), interim (no reviewer is ever asked for one -> workflow.needsReview, so it is the
// user's from the moment it opens), or open with no reviewer alive anywhere. A rejected PR waits on
// its author. Liveness is fleet-wide: an unassigned PR is ordinary while a reviewer runs, and only
// the absence of every live one strands the queue; one up but stuck is the Agents marker's.
func PRNeedsUser(p PR, agents []AgentView) bool {
	if p.Status == "approved" {
		return true
	}
	if p.Status != "open" {
		return false
	}
	return p.Kind == "interim" || !AnyLiveReviewer(agents)
}

// AnyLiveReviewer reports whether a reviewer agent is up anywhere in the fleet.
func AnyLiveReviewer(agents []AgentView) bool {
	for _, a := range agents {
		if a.Role == "reviewer" && !AgentNotUp(a.Status) {
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
