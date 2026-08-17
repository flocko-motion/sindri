package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// mailAgent seeds a worker holding a task, the shape every sender delivers to.
func mailAgent(t *testing.T) (*Hub, *store.ProjectStore) {
	t.Helper()
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-1", Title: "a task", Status: "open", Priority: "P1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "td-1", Branch: "td-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	return h, ps
}

// TestMailIsKeptForAnAgentThatCannotBeReached is the gap the whole feature exists to close: the agent
// is not running, so the push cannot land — and the message survives anyway, with pushed still false
// so a reader can see it was never delivered live.
func TestMailIsKeptForAnAgentThatCannotBeReached(t *testing.T) {
	h, ps := mailAgent(t)
	if err := h.Deliver(testProject, "dvalin", "[reviewer] rejected: the gate is missing",
		workflow.MailAndPush.From("reviewer")); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	unread, err := ps.UnreadMail("dvalin")
	if err != nil || len(unread) != 1 {
		t.Fatalf("the message must be waiting, got %+v (err %v)", unread, err)
	}
	if unread[0].Pushed {
		t.Error("the agent is down, so no push landed — the flag must say so")
	}
	// The sender is what the SENDER stated (-> workflow.Delivery.From), not what the body's tag happens
	// to say — that inference is what sd-bc3a1f removed, and only two senders could survive it.
	if unread[0].Sender != "reviewer" {
		t.Errorf("sender = %q, want the one the delivery stated", unread[0].Sender)
	}
}

// TestPushOnlyKeepsNothing: waking is the entire value of a nudge or a broadcast, so a wake nobody
// received is not a loss and must not become a permanent row. This is what makes keeping mail for
// ever affordable, so it is the half worth pinning hardest.
func TestPushOnlyKeepsNothing(t *testing.T) {
	h, _ := mailAgent(t)
	if err := h.Deliver(testProject, "dvalin", "[hub] carry on with td-1", workflow.PushOnly); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	all, err := h.store.AllMail(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("push-only traffic must not be stored, got %+v", all)
	}
}

// TestADeliveryThatSendsNothingIsAFault: neither property set is not a message. Answering it silently
// would let a sender that forgot to classify look like one that chose not to send.
func TestADeliveryThatSendsNothingIsAFault(t *testing.T) {
	h, _ := mailAgent(t)
	if err := h.Deliver(testProject, "dvalin", "nothing", workflow.Delivery{}); err == nil {
		t.Error("a delivery asking for neither mail nor push should be refused")
	}
}

// TestTheDirectiveRemindsAndReadingClearsIt: `sindri` is the one place every agent already looks, and
// the reminder REPLACES the ordinary directive because mail can change what the next action is — a
// cancellation for the task it holds, say. After reading, the directive is the work again.
func TestTheDirectiveRemindsAndReadingClearsIt(t *testing.T) {
	h, ps := mailAgent(t)
	if _, err := ps.AddMail("dvalin", "hub", "[hub] td-1 was cancelled", false); err != nil {
		t.Fatal(err)
	}
	dir, err := h.wf.AgentDirective(t.Context(), testProject, "dvalin")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1 unread", "sindri mail"} {
		if !strings.Contains(dir, want) {
			t.Errorf("the directive should name %q: %q", want, dir)
		}
	}
	if strings.Contains(dir, "Work on task") {
		t.Errorf("mail comes first, since it may change what the next action is: %q", dir)
	}

	out, code := execAs(t, h, "dvalin", "mail")
	if code != 0 {
		t.Fatalf("mail failed (%d): %s", code, out)
	}
	if !strings.Contains(out, "td-1 was cancelled") {
		t.Errorf("the verb should hand over the message: %s", out)
	}
	// Relevance is settled at pickup, not by a clock: the reply sends the reader back to live state.
	if !strings.Contains(out, "CHECK EACH ONE") {
		t.Errorf("the reply should make the reader check relevance: %s", out)
	}
	// Read, not deleted — the record survives for a human.
	if n, _ := ps.UnreadMailCount("dvalin"); n != 0 {
		t.Errorf("reading should leave nothing unread, got %d", n)
	}
	if all, _ := h.store.AllMail(0); len(all) != 1 || !all[0].Read() {
		t.Errorf("the message must remain, marked read: %+v", all)
	}
	// And the directive is the work again.
	dir, err = h.wf.AgentDirective(t.Context(), testProject, "dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, "td-1") || strings.Contains(dir, "unread") {
		t.Errorf("with the mailbox empty the directive is the task again: %q", dir)
	}
}

// TestAnEscalatedAgentIsStillToldItHasMail is the narrow path this feature exists for. An escalated
// agent is the one state explicitly instructed to sit and wait, so it is the LAST that would discover
// mail by chance — and the mail it is waiting on may be the answer, or may moot the task it asked
// about. The escalation directive also claims nothing has come back, which is only true once the
// mailbox is empty, so mail is answered first and the claim becomes true by construction.
func TestAnEscalatedAgentIsStillToldItHasMail(t *testing.T) {
	h, ps := mailAgent(t)
	if _, err := h.Escalate(testProject, "dvalin", "one column or two?"); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddMail("dvalin", "reviewer", "[reviewer] rejected: see the findings", false); err != nil {
		t.Fatal(err)
	}
	dir, err := h.wf.AgentDirective(t.Context(), testProject, "dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("an escalated agent with mail must be told about it: %q", dir)
	}
	if strings.Contains(dir, "Nothing has come back yet") {
		t.Errorf("it must not be told nothing has come back while a message waits: %q", dir)
	}
	// Reading it is never held back by the escalation, and afterwards the escalation directive is the
	// answer again — now truthfully, since the mailbox is empty.
	if out, code := execAs(t, h, "dvalin", "mail"); code != 0 {
		t.Fatalf("an escalated agent must be able to read its mail (%d): %s", code, out)
	}
	dir, err = h.wf.AgentDirective(t.Context(), testProject, "dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, "ESCALATED") || !strings.Contains(dir, "one column or two?") {
		t.Errorf("with the mailbox empty it goes back to waiting on its question: %q", dir)
	}
	if !strings.Contains(dir, "mailbox is empty") {
		t.Errorf("and the claim it makes about the mailbox should be the checked one: %q", dir)
	}
}

// TestAnEmptyMailboxSaysSo: an agent told to check its mail and shown nothing must be able to tell
// "nothing is waiting" from "something went wrong".
func TestAnEmptyMailboxSaysSo(t *testing.T) {
	h, _ := mailAgent(t)
	out, code := execAs(t, h, "dvalin", "mail")
	if code != 0 {
		t.Fatalf("mail failed (%d): %s", code, out)
	}
	if !strings.Contains(out, "No unread messages") {
		t.Errorf("an empty mailbox should say so plainly: %s", out)
	}
}

// TestUnreadShowsOnTheAgentRow: a mailbox waits quietly by design, so a count on the agent is the
// only thing that reveals one has stopped reading.
func TestUnreadShowsOnTheAgentRow(t *testing.T) {
	h, ps := mailAgent(t)
	if _, err := ps.AddMail("dvalin", "hub", "[hub] read me", false); err != nil {
		t.Fatal(err)
	}
	board, err := h.State("")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range board.Agents {
		if a.Name == "dvalin" && a.UnreadMail != 1 {
			t.Errorf("the agent row should carry its unread count, got %d", a.UnreadMail)
		}
	}
}
