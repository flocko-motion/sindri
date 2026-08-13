// package: api / attention
// type:    logic (a rule over the board)
// job:     which agents cannot move without the user — the rule behind every
// front-end's attention marker, written once so all of them count one set.
// limits:  a pure rule over AgentView; the section registry that calls it is hub-side.
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
// keeps running, so it fills up or stalls — and a marker on one would sit there until it was
// deleted. The stall nudge exempts it for the same reason (-> workflow.parkedByTheHub).
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
