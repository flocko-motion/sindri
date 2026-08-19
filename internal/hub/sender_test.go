package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// TestTheSenderIsStatedNotSniffed is the groundwork this task exists for: provenance used to be read
// out of a "[user] " prefix in the body, so it was a property of how a message happened to be worded —
// and only the hub and the user could ever be recorded. A sender states who it is.
func TestTheSenderIsStatedNotSniffed(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		what   string
		text   string
		d      workflow.Delivery
		expect string
	}{
		// Each of the four the wire type documents, including the two that were unreachable.
		{"the hub in its own voice", "[hub] merged", workflow.MailOnly, "hub"},
		{"a reviewer's verdict", "[reviewer] rejected: thin tests", workflow.MailOnly.From("reviewer"), "reviewer"},
		{"the user", "[user] have a look", workflow.MailOnly.From(api.SenderUser), api.SenderUser},
		{"another agent", "the adapter shells out twice", workflow.MailOnly.From("nori"), "nori"},
		// And the tag in the text no longer decides anything: this one says user and is from an agent.
		{"a tagged body from an agent", "[user] quoted text", workflow.MailOnly.From("gloin"), "gloin"},
	} {
		if err := h.Deliver(testProject, "dvalin", c.text, c.d); err != nil {
			t.Fatalf("%s: %v", c.what, err)
		}
	}
	mail, err := h.store.AllMail(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(mail) != 5 {
		t.Fatalf("expected five messages, got %d", len(mail))
	}
	// Newest first, so the expectations read in reverse.
	want := []string{"gloin", "nori", api.SenderUser, "reviewer", "hub"}
	for i, w := range want {
		if mail[i].Sender != w {
			t.Errorf("message %d: sender = %q, want %q (body %q)", i, mail[i].Sender, w, mail[i].Body)
		}
	}
}

// TestAnAgentsNoteGoesThroughTheOneDeliveryPath: fyi used to write to the store itself, which is the
// parallel sender path this task warned about. It now goes through Deliver like everything else, with
// the agent stated as the sender — and no push, since the user has no session to type into.
func TestAnAgentsNoteGoesThroughTheOneDeliveryPath(t *testing.T) {
	h, ps := noteSender(t, "dvalin")
	if out, code := execAs(t, h, "dvalin", "fyi", "the adapter shells out twice per sync"); code != 0 {
		t.Fatalf("fyi failed (%d): %s", code, out)
	}
	mail, _ := h.store.AllMail(0)
	if len(mail) != 1 {
		t.Fatalf("expected the note, got %d rows", len(mail))
	}
	if mail[0].Agent != api.SenderUser || mail[0].Sender != "dvalin" {
		t.Errorf("recipient/sender = %q/%q, want user/dvalin", mail[0].Agent, mail[0].Sender)
	}
	if mail[0].Pushed {
		t.Error("the user has no session, so nothing should record a wake")
	}
	_ = ps
}

// TestDeliveringToTheUserNeverAttemptsAPush: a push to the user is not a delivery that failed, it is
// one that does not exist — so it must not be attempted, reported, or waited on.
func TestDeliveringToTheUserNeverAttemptsAPush(t *testing.T) {
	h := newHub(t)
	if err := h.Deliver(testProject, api.SenderUser, "a note", workflow.MailAndPush.From("nori")); err != nil {
		t.Fatalf("delivering to the user should not fail: %v", err)
	}
	mail, _ := h.store.AllMail(0)
	if len(mail) != 1 || mail[0].Pushed {
		t.Errorf("expected one unpushed row, got %+v", mail)
	}
}

// TestEveryDeliveryClassCarriesItsSenderUnchanged: From returns a COPY, so naming a sender on one call
// cannot leak into the shared classification values every other call site uses.
func TestEveryDeliveryClassCarriesItsSenderUnchanged(t *testing.T) {
	tagged := workflow.MailAndPush.From("reviewer")
	if tagged.Sender != "reviewer" {
		t.Fatalf("From should name the sender, got %q", tagged.Sender)
	}
	if workflow.MailAndPush.Sender != "" {
		t.Errorf("the shared classification was mutated: %q", workflow.MailAndPush.Sender)
	}
	if !tagged.Mail || !tagged.Push {
		t.Error("naming a sender must not change how the message travels")
	}
}

// TestTheRejectionSaysWhoRejectedIt walks a real sender end to end: the feedback is the reviewer's or
// the user's, and an agent weights a message by who it is from.
func TestTheRejectionSaysWhoRejectedIt(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-td-1", Task: "td-1", Agent: "dvalin", Branch: "td-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-1", Branch: "td-1", Phase: "submitted"}); err != nil {
		t.Fatal(err)
	}
	if err := h.wf.RejectPR(testProject, "pr-td-1", "the gate is missing"); err != nil {
		t.Fatalf("RejectPR: %v", err)
	}
	mail, _ := h.store.AllMail(0)
	if len(mail) == 0 {
		t.Fatal("the rejection should have been mailed")
	}
	if mail[0].Sender != api.SenderUser {
		t.Errorf("a human rejection is from the user, got %q", mail[0].Sender)
	}
	// A pointer, not a third copy of the feedback: pr.Feedback is canonical, and DirRejected already
	// re-serves it verbatim on every ask while the PR stays rejected.
	if strings.Contains(mail[0].Body, "the gate is missing") {
		t.Errorf("mail should point at `sindri`, not duplicate the feedback: %q", mail[0].Body)
	}
	if !strings.Contains(mail[0].Body, "sindri") {
		t.Errorf("mail should point the reader at `sindri` for the feedback: %q", mail[0].Body)
	}
	if pr, _, _ := ps.GetPR("pr-td-1"); pr.Feedback != "the gate is missing" {
		t.Errorf("the canonical feedback should live on the PR, got %q", pr.Feedback)
	}
}
