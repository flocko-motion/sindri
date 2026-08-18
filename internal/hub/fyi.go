// package: hub / fyi
// type:    logic (the agent-to-user note channel and its budget)
// job:     let an agent tell the user something it noticed in passing, and bound how much of
// that the user must read — a length cap, a grant per claim, a fleet-wide hourly
// ceiling. The numbers are named here, since the first values will be wrong.
// limits:  the policy and the verb; the mailbox is the store's, and every word the agent
// reads is workflow's (-> prompts_fyi.go).
package hub

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// The budget. Work-based rather than time-based on purpose: time accrues while an agent sits idle, so
// it rewards having seen nothing and lets a long-blocked agent wake with a full purse. A grant per
// CLAIM ties the right to speak to having been somewhere and looked at something.
const (
	// maxNoteLen is the shared delivery cap (-> deliver.go): what a reader has to read is the same cost
	// whoever they are, so the length rule is not the note channel's own.
	maxNoteLen = maxMessageLen
	// notesPerClaim is workflow's, since the claim paths that grant it live there — named here too so
	// the four numbers read together (-> workflow.NotesPerClaim).
	notesPerClaim = workflow.NotesPerClaim
	// fleetNotesPerHour is what actually protects the user, and the reason a per-agent grant is not
	// enough: a work-based budget scales with the fleet and the user does not. Forty agents each
	// behaving impeccably still bury one person, every individual decision along the way correct.
	fleetNotesPerHour = 6
	// fleetNoteWindow is the span that ceiling is counted over — rolling, so there is no cliff at the
	// top of the hour for a queue to build against.
	fleetNoteWindow = time.Hour
)

// fyiUsage is what an empty `fyi` is answered with: the whole register, since the failure mode is
// sending the wrong kind of thing rather than mistyping the verb.
var fyiUsage = "usage: fyi <what you noticed>\n" + workflow.FyiGuidance

// cmdFyi is the agent-facing `fyi <message...>` verb: one short note to the user, against a budget.
//
// The three refusals are deliberate and each names where the material actually belongs. Refusing is
// also the honest answer at the fleet ceiling: holding the note would preserve it at the cost of
// delivering it hours stale, into a queue the user cannot see. Every refusal is LOGGED, because those
// counts are the only evidence for what these numbers should be.
func (h *Hub) cmdFyi(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := h.store.For(c.Project)
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		fmt.Fprintln(out, fyiUsage)
		return 2, nil
	}
	if n := len([]rune(msg)); n > maxNoteLen {
		_ = ps.Log(c.Agent, "fyi-refused", fmt.Sprintf("too long: %d of %d chars", n, maxNoteLen))
		fmt.Fprintln(out, workflow.ReplyFyiTooLong(n, maxNoteLen))
		return 1, nil
	}
	left, err := ps.NotesLeft(c.Agent)
	if err != nil {
		return 1, err
	}
	if left <= 0 {
		_ = ps.Log(c.Agent, "fyi-refused", "grant spent on this claim")
		fmt.Fprintln(out, workflow.ReplyFyiSpent(notesPerClaim))
		return 1, nil
	}
	sent, err := h.store.NotesToUserSince(time.Now().Add(-fleetNoteWindow))
	if err != nil {
		return 1, err
	}
	if sent >= fleetNotesPerHour {
		// Logged with the count, since this is the refusal whose frequency decides whether the ceiling
		// is right — and the one that can kill a note somebody else's chatter crowded out.
		_ = ps.Log(c.Agent, "fyi-refused", fmt.Sprintf("fleet ceiling: %d in the last %s", sent, fleetNoteWindow))
		fmt.Fprintln(out, workflow.ReplyFyiFleetFull(fleetNotesPerHour, fleetNoteWindow))
		return 1, nil
	}
	// Through the one delivery path, with the agent as an explicit sender — which is the third of the
	// four senders Mail.Sender documents, and was unreachable while provenance was sniffed from text.
	if err := h.Deliver(c.Project, api.SenderUser, msg, workflow.MailOnly.From(c.Agent)); err != nil {
		return 1, err
	}
	if err := ps.SetNotesLeft(c.Agent, left-1); err != nil {
		return 1, err
	}
	_ = ps.Log(c.Agent, "fyi", msg)
	h.notify()
	fmt.Fprintln(out, workflow.ReplyFyiSent(left-1))
	return 0, nil
}
