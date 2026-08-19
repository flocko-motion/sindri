package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestReadingTheUsersMailMarksItRead is sd-1d2213 part B: a message addressed to the user is the one
// case where a human LOOKING at it via MailBody is the reading the mailbox promises to record.
func TestReadingTheUsersMailMarksItRead(t *testing.T) {
	h := newHub(t)
	m, err := h.store.For(testProject).AddMail(api.SenderUser, "dvalin", "the gate keeps flaking", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if m.Read() {
		t.Fatal("precondition: the message starts unread")
	}
	got, ok, err := h.MailBody(m.ID)
	if err != nil || !ok {
		t.Fatalf("MailBody(%d): ok=%v err=%v", m.ID, ok, err)
	}
	if !got.Read() {
		t.Error("MailBody should mark the user's own mail read and reflect that in its own return")
	}
	stored, ok, err := h.store.MailByID(m.ID)
	if err != nil || !ok {
		t.Fatalf("MailByID(%d): ok=%v err=%v", m.ID, ok, err)
	}
	if !stored.Read() {
		t.Error("the mark must actually be written, not just reflected in the one response")
	}
}

// TestReadingAnAgentsMailDoesNotMarkIt is the hazard the ticket named: MailBody is also how a human
// inspects an AGENT's mailbox, and marking that read on a mere look would tell AgentDirective the
// message was consumed before the agent ever asked — swallowing it at the point it exists to interrupt.
func TestReadingAnAgentsMailDoesNotMarkIt(t *testing.T) {
	h := newHub(t)
	m, err := h.store.For(testProject).AddMail("dvalin", "reviewer", "rejected: needs another pass", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := h.MailBody(m.ID); err != nil || !ok {
		t.Fatalf("MailBody(%d): ok=%v err=%v", m.ID, ok, err)
	}
	stored, ok, err := h.store.MailByID(m.ID)
	if err != nil || !ok {
		t.Fatalf("MailByID(%d): ok=%v err=%v", m.ID, ok, err)
	}
	if stored.Read() {
		t.Error("a human looking at an agent's mail must not mark it read out from under the agent")
	}
}
