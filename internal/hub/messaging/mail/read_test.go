package mail

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestMailBodyNeverMarksAnything: a look is not a consequence. sd-70dad1 replaced "viewing marks
// read" (ENTER, then even a bare cursor move) with a deliberate act — a dwell, an ENTER on a narrow
// terminal, or `mail show` — so the fetch itself must be inert, whoever the message is addressed to.
func TestMailBodyNeverMarksAnything(t *testing.T) {
	b, ps, _ := newBox(t)
	toUser, err := ps.AddMail(api.SenderUser, "dvalin", "the gate keeps flaking", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	toAgent, err := ps.AddMail("dvalin", "reviewer", "rejected: needs another pass", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{toUser.ID, toAgent.ID} {
		if got, ok, err := b.MailBody(id); err != nil || !ok || got.Read() {
			t.Errorf("MailBody(%d) must not mark it read: got=%+v ok=%v err=%v", id, got, ok, err)
		}
		if stored, ok, err := b.store.MailByID(id); err != nil || !ok || stored.Read() {
			t.Errorf("mail %d was marked read by a mere fetch", id)
		}
	}
}

// TestMarkMailReadForUserMarksOnlyMailToTheUser is the scope guarantee sd-70dad1 exists to protect:
// the deliberate mark applies to the user's own mail and is a silent no-op for an agent's, so a
// human browsing the fleet's mailbox can never consume another agent's delivery.
func TestMarkMailReadForUserMarksOnlyMailToTheUser(t *testing.T) {
	b, ps, _ := newBox(t)
	toUser, err := ps.AddMail(api.SenderUser, "dvalin", "the gate keeps flaking", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	toAgent, err := ps.AddMail("dvalin", "reviewer", "rejected: needs another pass", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.MarkMailReadForUser(toUser.ID); err != nil {
		t.Fatalf("MarkMailReadForUser(%d): %v", toUser.ID, err)
	}
	if stored, _, _ := b.store.MailByID(toUser.ID); !stored.Read() {
		t.Error("mail addressed to the user should now be marked read")
	}
	if err := b.MarkMailReadForUser(toAgent.ID); err != nil {
		t.Fatalf("MarkMailReadForUser(%d): %v", toAgent.ID, err)
	}
	if stored, _, _ := b.store.MailByID(toAgent.ID); stored.Read() {
		t.Error("mail addressed to an agent must never be marked read out from under it")
	}
}
