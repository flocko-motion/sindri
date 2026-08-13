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
// Idle never counts — an agent waiting for work is functioning normally, and one idle beside work
// it could claim is the hub's fault to nudge rather than the user's to fix. Retired is a decision
// already taken. An api-error is the hub's to resend, and once resending stops working the screen
// stands still and the board says stalled.
func AgentNeedsUser(a AgentView) bool {
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
