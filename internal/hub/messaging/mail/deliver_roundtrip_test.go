// package: hub/messaging/mail / roundtrip_test
// type:    logic (a message from one end to the other)
// job:     drive the mailbox the way its callers do — a send, a push that cannot land, a reply
// threaded back — so what survives, what is announced and who it says it is from are checked
// against the store rather than against a mock.
// limits:  the mailbox. What a DIRECTIVE does with waiting mail, and what the board counts, are
// tested where those live.
package mail

import (
	"fmt"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestMailIsKeptForAnAgentThatCannotBeReached is the gap the whole feature exists to close: the agent
// is not running, so the push cannot land — and the message survives anyway, with pushed still false
// so a reader can see it was never delivered live.
func TestMailIsKeptForAnAgentThatCannotBeReached(t *testing.T) {
	b, ps := mailAgent(t)
	if err := b.Deliver(testProject, "dvalin", "[reviewer] rejected: the gate is missing",
		MailAndPush.From("reviewer")); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	b.Settle() // the push runs behind the written mail; this reads what it recorded
	unread, err := ps.UnreadMail("dvalin")
	if err != nil || len(unread) != 1 {
		t.Fatalf("the message must be waiting, got %+v (err %v)", unread, err)
	}
	if unread[0].Pushed {
		t.Error("the agent is down, so no push landed — the flag must say so")
	}
	// The sender is what the SENDER stated (-> Delivery.From), not what the body's tag happens
	// to say — that inference is what sd-bc3a1f removed, and only two senders could survive it.
	if unread[0].Sender != "reviewer" {
		t.Errorf("sender = %q, want the one the delivery stated", unread[0].Sender)
	}
}

// TestPushOnlyKeepsNothing: waking is the entire value of a nudge or a broadcast, so it must not
// become a permanent row. This is what makes keeping mail for ever affordable, so it is the half
// worth pinning hardest. The push is REPORTED though — with no row behind it the push IS the
// message, and a swallowed failure left hepti's mailbox marked announced to nobody.
func TestPushOnlyKeepsNothing(t *testing.T) {
	b, ps := mailAgent(t)
	_ = ps
	if err := b.Deliver(testProject, "dvalin", "[hub] carry on with td-1", PushOnly); err == nil {
		t.Error("a push-only delivery to an agent with no session reported success; its caller cannot retry")
	}
	all, err := b.store.AllMail(0)
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
	b, ps := mailAgent(t)
	_ = ps
	if err := b.Deliver(testProject, "dvalin", "nothing", Delivery{}); err == nil {
		t.Error("a delivery asking for neither mail nor push should be refused")
	}
}

// TestAnEmptyMailboxSaysSo: an agent told to check its mail and shown nothing must be able to tell
// "nothing is waiting" from "something went wrong".
func TestAnEmptyMailboxSaysSo(t *testing.T) {
	b, ps := mailAgent(t)
	_ = ps
	out, code := verbAs(t, b, ps, "dvalin", "mail")
	if code != 0 {
		t.Fatalf("mail failed (%d): %s", code, out)
	}
	if !strings.Contains(out, "No unread messages") {
		t.Errorf("an empty mailbox should say so plainly: %s", out)
	}
}

// TestTheSenderIsStatedNotSniffed is the groundwork this task exists for: provenance used to be read
// out of a "[user] " prefix in the body, so it was a property of how a message happened to be worded —
// and only the hub and the user could ever be recorded. A sender states who it is.
func TestTheSenderIsStatedNotSniffed(t *testing.T) {
	b, ps := newBox2(t)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		what   string
		text   string
		d      Delivery
		expect string
	}{
		// Each of the four the wire type documents, including the two that were unreachable.
		{"the hub in its own voice", "[hub] merged", MailOnly, "hub"},
		{"a reviewer's verdict", "[reviewer] rejected: thin tests", MailOnly.From("reviewer"), "reviewer"},
		{"the user", "[user] have a look", MailOnly.From(api.SenderUser), api.SenderUser},
		{"another agent", "the adapter shells out twice", MailOnly.From("nori"), "nori"},
		// And the tag in the text no longer decides anything: this one says user and is from an agent.
		{"a tagged body from an agent", "[user] quoted text", MailOnly.From("gloin"), "gloin"},
	} {
		if err := b.Deliver(testProject, "dvalin", c.text, c.d); err != nil {
			t.Fatalf("%s: %v", c.what, err)
		}
	}
	mail, err := b.store.AllMail(0)
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
	b, ps := noteSender(t, "dvalin")
	if out, code := verbAs(t, b, ps, "dvalin", "fyi", "the adapter shells out twice per sync"); code != 0 {
		t.Fatalf("fyi failed (%d): %s", code, out)
	}
	mail, _ := b.store.AllMail(0)
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
	b, ps := newBox2(t)
	_ = ps
	if err := b.Deliver(testProject, api.SenderUser, "a note", MailAndPush.From("nori")); err != nil {
		t.Fatalf("delivering to the user should not fail: %v", err)
	}
	mail, _ := b.store.AllMail(0)
	if len(mail) != 1 || mail[0].Pushed {
		t.Errorf("expected one unpushed row, got %+v", mail)
	}
}

// TestEveryDeliveryClassCarriesItsSenderUnchanged: From returns a COPY, so naming a sender on one call
// cannot leak into the shared classification values every other call site uses.
func TestEveryDeliveryClassCarriesItsSenderUnchanged(t *testing.T) {
	tagged := MailAndPush.From("reviewer")
	if tagged.Sender != "reviewer" {
		t.Fatalf("From should name the sender, got %q", tagged.Sender)
	}
	if MailAndPush.Sender != "" {
		t.Errorf("the shared classification was mutated: %q", MailAndPush.Sender)
	}
	if !tagged.Mail || !tagged.Push {
		t.Error("naming a sender must not change how the message travels")
	}
}

// TestMailingAnUnknownAgentIsRefused: a name with no roster entry is a typo, and mail is durable —
// silently keeping a message for an agent that does not exist would hide the mistake for ever.
func TestMailingAnUnknownAgentIsRefused(t *testing.T) {
	b, ps := mailAgent(t)
	_ = ps
	if err := b.MailAgent(testProject, "nobody", "hello"); err == nil {
		t.Error("mailing an agent that does not exist should be refused")
	}
	if all, _ := b.store.AllMail(0); len(all) != 0 {
		t.Errorf("nothing should have been stored: %+v", all)
	}
}

// TestMailReachesASignedOutAgent is the case sd-f66d3b's refusal left behind: text typed at a /login
// prompt vanishes, so `tell` asks about it — but mail never touches the session, so it simply works
// and is read when the session recovers. The guard belongs to the push path alone.
func TestMailReachesASignedOutAgent(t *testing.T) {
	b, ps := newBox2(t)
	if err := ps.PutAgent(store.Agent{Name: "gloin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	// No pod at all, which is the strongest form of "cannot be typed into".
	if err := b.MailAgent(testProject, "gloin", "the token is renewed, carry on"); err != nil {
		t.Fatalf("mail must not depend on a live session: %v", err)
	}
	if n, _ := ps.UnreadMailCount("gloin"); n != 1 {
		t.Errorf("unread = %d, want the message waiting", n)
	}
}

// TestReplyingNeedsNoNameAndThreads is the whole point: an agent that has been mailed can answer
// without being told who wrote to it, so a conversation needs no directory at either end — and the
// answer records what it answers, so the recipient is not left matching it to its own messages.
func TestReplyingNeedsNoNameAndThreads(t *testing.T) {
	b, ps := twoRepos(t)
	if out, code := verbAs(t, b, ps, "dvalin", "mail", "galar", "the adapter shells out twice per sync"); code != 0 {
		t.Fatalf("mail failed (%d): %s", code, out)
	}
	sent, err := b.store.For("lib").UnreadMail("galar")
	if err != nil || len(sent) != 1 {
		t.Fatalf("expected the message, got %+v (err %v)", sent, err)
	}

	out, code := verbIn(t, b, "lib", "galar", "reply", fmt.Sprint(sent[0].ID), "it does; the second call is cached upstream")
	if code != 0 {
		t.Fatalf("reply failed (%d): %s", code, out)
	}
	back, err := b.store.For(testProject).UnreadMail("dvalin")
	if err != nil || len(back) != 1 {
		t.Fatalf("the reply should reach the original sender, got %+v (err %v)", back, err)
	}
	if back[0].InReplyTo != sent[0].ID {
		t.Errorf("in_reply_to = %d, want the message it answers (%d)", back[0].InReplyTo, sent[0].ID)
	}
	if !strings.HasSuffix(back[0].Sender, "/galar") {
		t.Errorf("the reply is attributed to its writer, qualified: %q", back[0].Sender)
	}
	if back[0].Pushed {
		t.Error("a reply is mail — it does not wake anyone")
	}
}

// TestAReplyToTheHubIsRefusedWithSomewhereToGo: the hub is not a correspondent. An answer typed at a
// notification would be read by nobody, so the refusal names the paths that do reach someone.
func TestAReplyToTheHubIsRefusedWithSomewhereToGo(t *testing.T) {
	b, ps := twoRepos(t)
	m, err := ps.AddMail("dvalin", "hub", "[hub] merged pr-td-1", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, code := verbAs(t, b, ps, "dvalin", "reply", fmt.Sprint(m.ID), "thanks")
	if code == 0 {
		t.Errorf("replying to the hub must be refused: %s", out)
	}
	for _, want := range []string{"sindri escalate", "sindri comment"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal should name %q as a path that reaches someone: %s", want, out)
		}
	}
	if all, _ := b.store.AllMail(0); len(all) != 1 {
		t.Errorf("nothing should have been sent: %d rows", len(all))
	}
}

// TestReplyingToTheUserIsNotCharged: the note grant bounds attention the user did not ask for, and a
// reply answers a message they chose to send. Charging it would penalise answering and teach agents to
// go quiet when addressed directly.
func TestReplyingToTheUserIsNotCharged(t *testing.T) {
	b, ps := noteSender(t, "dvalin")
	// Spend the whole grant first, so any charge would refuse the reply.
	for i := 0; i < prompts.NotesPerClaim; i++ {
		if out, code := verbAs(t, b, ps, "dvalin", "fyi", "something worth knowing"); code != 0 {
			t.Fatalf("note %d refused early (%d): %s", i+1, code, out)
		}
	}
	if n, _ := ps.NotesLeft("dvalin"); n != 0 {
		t.Fatalf("precondition: the grant should be spent, got %d left", n)
	}
	asked, err := ps.AddMail("dvalin", api.SenderUser, "[user] which of the two callers matters?", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, code := verbAs(t, b, ps, "dvalin", "reply", fmt.Sprint(asked.ID), "the second one; the first is dead code")
	if code != 0 {
		t.Fatalf("a reply to the user must not be charged to the note grant (%d): %s", code, out)
	}
	mine, err := ps.UnreadMail(api.SenderUser)
	if err != nil || len(mine) != 3 { // two notes plus the reply
		t.Fatalf("the reply should reach the user's mailbox, got %d rows (err %v)", len(mine), err)
	}
	if mine[2].InReplyTo != asked.ID {
		t.Errorf("the reply should be threaded, got in_reply_to %d", mine[2].InReplyTo)
	}
}

// TestYouCanOnlyReplyToYourOwnMail: an id is not a licence to read another agent's mailbox, and a reply
// to somebody else's message would be an answer to a question its recipient never saw.
func TestYouCanOnlyReplyToYourOwnMail(t *testing.T) {
	b, ps := twoRepos(t)
	m, err := b.store.For("lib").AddMail("galar", "someone/else", "not for you", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, code := verbAs(t, b, ps, "dvalin", "reply", fmt.Sprint(m.ID), "answering somebody else's mail")
	if code == 0 || !strings.Contains(out, "in your mailbox") {
		t.Errorf("expected a refusal naming whose mailbox it is (%d): %s", code, out)
	}
}

// TestTheUserRepliesFromWhereTheyAreReading is the other half: the recipient comes from the row, so
// neither front-end asks who wrote it.
func TestTheUserRepliesFromWhereTheyAreReading(t *testing.T) {
	b, ps := noteSender(t, "dvalin")
	if out, code := verbAs(t, b, ps, "dvalin", "fyi", "the config field is documented backwards"); code != 0 {
		t.Fatalf("fyi failed (%d): %s", code, out)
	}
	note, _ := ps.UnreadMail(api.SenderUser)
	if len(note) != 1 {
		t.Fatalf("expected the note, got %d", len(note))
	}
	if err := b.ReplyToMail(note[0].ID, "fix it in the same PR then"); err != nil {
		t.Fatalf("ReplyToMail: %v", err)
	}
	back, _ := ps.UnreadMail("dvalin")
	if len(back) != 1 || back[0].Sender != api.SenderUser {
		t.Fatalf("the reply should reach the agent, from the user: %+v", back)
	}
	if back[0].InReplyTo != note[0].ID || back[0].Pushed {
		t.Errorf("threaded and unpushed expected, got in_reply_to %d pushed %v", back[0].InReplyTo, back[0].Pushed)
	}
	// And the hub is not a correspondent from this side either.
	hubMsg, _ := ps.AddMail("dvalin", "hub", "[hub] merged", true, 0)
	if err := b.ReplyToMail(hubMsg.ID, "thanks"); err == nil {
		t.Error("replying to a hub notification should be refused for the user too")
	}
	_ = store.Agent{}
}

// TestAnAgentSeesTheIdsItCanReplyTo is the gap this rendering exposed: reading mail is the only place an
// agent learns an id, so the read half has to show them — otherwise `reply <mail-id>` is a verb whose
// argument the agent can never obtain. Both spellings are accepted, since the bare one is what anything
// already told to an agent contains.
func TestAnAgentSeesTheIdsItCanReplyTo(t *testing.T) {
	b, ps := twoRepos(t)
	m, err := ps.AddMail("dvalin", "lib/galar", "the second call is cached upstream", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, code := verbAs(t, b, ps, "dvalin", "mail")
	if code != 0 {
		t.Fatalf("mail failed (%d): %s", code, out)
	}
	if !strings.Contains(out, api.MailID(m.ID)) {
		t.Errorf("reading mail should show each id, so a reply has something to name: %s", out)
	}

	// The prefixed form works...
	if out, code := verbIn(t, b, testProject, "dvalin", "reply", api.MailID(m.ID), "understood"); code != 0 {
		t.Fatalf("a prefixed id should be accepted (%d): %s", code, out)
	}
	// ...and so does the bare one, which is what older messages and shell history carry.
	m2, err := ps.AddMail("dvalin", "lib/galar", "one more thing", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out, code := verbIn(t, b, testProject, "dvalin", "reply", fmt.Sprint(m2.ID), "also understood"); code != 0 {
		t.Fatalf("a bare id must keep working (%d): %s", code, out)
	}
	// And a refusal shows the shape rather than just saying no.
	out, code = verbAs(t, b, ps, "dvalin", "reply", "nonsense", "hello")
	if code == 0 || !strings.Contains(out, api.MailIDPrefix) {
		t.Errorf("the refusal should show what an id looks like (%d): %s", code, out)
	}
}
