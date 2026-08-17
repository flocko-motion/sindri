// package: hub/workflow / prompts fyi
// type:    logic (the agent-facing strings for the note channel)
// job:     what an agent reads about `fyi` — the register the verb is for, what it has left,
// and each refusal. The guidance leads with the NEGATIVES because the failure mode is
// over-sending, and it is repeated in every refusal rather than stated once.
// limits:  pure strings; the budget and its numbers are the hub's (-> hub/fyi.go).
package workflow

import (
	"fmt"
	"time"
)

// NotesPerClaim is the grant, given whole at each claim however long that claim runs: a three-day task
// has more to say than a twenty-minute one, but also far more places to say it. It lives here because
// the claim paths are the workflow's and the hub reads the same number for its refusals; expect the
// first value to be wrong, so changing it must stay a one-line edit.
const NotesPerClaim = 2

// FyiGuidance is the register, in the words every place it appears uses — the verb's help, the usage
// line, and each refusal. Negatives first: an agent that has just been refused is about to decide
// where the material goes instead, and this is the list that answers that.
const FyiGuidance = "  A note to the user about something you noticed IN PASSING. Not a request, not\n" +
	"  a report. The one-line test: IF YOU SAY NOTHING, DOES THIS FACT DISAPPEAR? If it\n" +
	"  does not, do not send.\n" +
	"    - NOT \"I finished X\" — the PR says that.\n" +
	"    - NOT a summary of the work — the PR and your log say that.\n" +
	"    - NOT a question or a blocker — that is `sindri escalate`.\n" +
	"    - NOT something about the task in hand — that is `sindri comment`, which the\n" +
	"      user reads beside the task.\n" +
	"    - YES: something with no home on this task or PR, that nobody will ever learn\n" +
	"      if you stay quiet."

// FyiHelp is the verb's help for a caller with left notes on this claim. The count is in the help on
// purpose: scarcity that is known induces far more selectivity than a cap discovered by hitting it,
// which turns the limit into a backstop rather than the mechanism.
func FyiHelp(left int) string {
	return fmt.Sprintf("tell the user one thing you noticed in passing (%s): fyi <message>\n%s",
		notesLeftPhrase(left), FyiGuidance)
}

// notesLeftPhrase says what is left in words rather than a bare number, since "none" has to read as a
// closed door rather than as a zero to try anyway.
func notesLeftPhrase(left int) string {
	switch left {
	case 0:
		return "none left on this claim"
	case 1:
		return "one note left on this claim"
	default:
		return fmt.Sprintf("%d notes left on this claim", left)
	}
}

// ReplyFyiSent confirms a note and states the remainder, so the next one is weighed against a known
// scarcity rather than sent and then refused.
func ReplyFyiSent(left int) string {
	if left == 0 {
		return "Noted — the user will read it. That was your last note on this claim; anything else " +
			"belongs in the PR body, a task comment, or an `escalate` if it blocks you."
	}
	return fmt.Sprintf("Noted — the user will read it. You have %s.", notesLeftPhrase(left))
}

// ReplyFyiTooLong refuses an over-length note. It says to CUT it and, explicitly, not to split it
// across two calls — which is the obvious workaround, and would spend a second note on the same point.
func ReplyFyiTooLong(n, max int) string {
	return fmt.Sprintf("Not sent: %d characters, and the limit is %d. CUT IT to the one fact that "+
		"would otherwise be lost — do NOT split it across two calls, which spends another note on the "+
		"same point and gives the user two things to read instead of one.\n%s", n, max, FyiGuidance)
}

// ReplyFyiSpent refuses once the claim's grant is gone, and names where the material goes instead. The
// grant REPLACES rather than accumulating, so the next claim starts whole however this one ended.
func ReplyFyiSpent(perClaim int) string {
	return fmt.Sprintf("Not sent: you have used both notes on this claim (%d per claim, granted whole "+
		"at each claim and never banked). This one belongs somewhere else: the PR body if it is about "+
		"the change, `sindri comment` if it is about the task, `sindri escalate` if it blocks you. Your "+
		"next claim starts with %d again.\n%s", perClaim, perClaim, FyiGuidance)
}

// ReplyFyiFleetFull refuses at the fleet-wide ceiling — the limit that exists because a per-agent
// budget scales with the fleet and the user's attention does not. It says plainly that the note is
// NOT held: a queue the user cannot see, delivered hours stale, would be worse than losing it.
func ReplyFyiFleetFull(perHour int, window time.Duration) string {
	return fmt.Sprintf("Not sent: the fleet has sent the user %d notes in the last %s, which is the "+
		"ceiling. It is refused rather than queued — a note held back and delivered hours later is "+
		"worse than one not sent. If it matters more than that, it is not an aside: put it in the PR "+
		"body, on the task with `sindri comment`, or `sindri escalate` it.\n%s",
		perHour, window, FyiGuidance)
}
