package hub

import (
	"fmt"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// execIn runs a verb as an agent of a NAMED project — execAs is fixed to the test project, and the
// point of this feature is that the two ends of a conversation sit in different repos.
func execIn(t *testing.T, h *Hub, project, agent string, args ...string) (string, int) {
	t.Helper()
	var out strings.Builder
	code, err := h.AgentExec(project, agent, args, &out)
	if err != nil {
		t.Fatalf("AgentExec %v as %s/%s: %v", args, project, agent, err)
	}
	return out.String(), code
}

// TestReplyingNeedsNoNameAndThreads is the whole point: an agent that has been mailed can answer
// without being told who wrote to it, so a conversation needs no directory at either end — and the
// answer records what it answers, so the recipient is not left matching it to its own messages.
func TestReplyingNeedsNoNameAndThreads(t *testing.T) {
	h := twoRepos(t)
	if out, code := execAs(t, h, "dvalin", "mail", "galar", "the adapter shells out twice per sync"); code != 0 {
		t.Fatalf("mail failed (%d): %s", code, out)
	}
	sent, err := h.store.For("lib").UnreadMail("galar")
	if err != nil || len(sent) != 1 {
		t.Fatalf("expected the message, got %+v (err %v)", sent, err)
	}

	out, code := execIn(t, h, "lib", "galar", "reply", fmt.Sprint(sent[0].ID), "it does; the second call is cached upstream")
	if code != 0 {
		t.Fatalf("reply failed (%d): %s", code, out)
	}
	back, err := h.store.For(testProject).UnreadMail("dvalin")
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
	h := twoRepos(t)
	ps := h.store.For(testProject)
	m, err := ps.AddMail("dvalin", "hub", "[hub] merged pr-td-1", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "dvalin", "reply", fmt.Sprint(m.ID), "thanks")
	if code == 0 {
		t.Errorf("replying to the hub must be refused: %s", out)
	}
	for _, want := range []string{"sindri escalate", "sindri comment"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal should name %q as a path that reaches someone: %s", want, out)
		}
	}
	if all, _ := h.store.AllMail(0); len(all) != 1 {
		t.Errorf("nothing should have been sent: %d rows", len(all))
	}
}

// TestReplyingToTheUserIsNotCharged: the note grant bounds attention the user did not ask for, and a
// reply answers a message they chose to send. Charging it would penalise answering and teach agents to
// go quiet when addressed directly.
func TestReplyingToTheUserIsNotCharged(t *testing.T) {
	h, ps := noteSender(t, "dvalin")
	// Spend the whole grant first, so any charge would refuse the reply.
	for i := 0; i < workflow.NotesPerClaim; i++ {
		if out, code := execAs(t, h, "dvalin", "fyi", "something worth knowing"); code != 0 {
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
	out, code := execAs(t, h, "dvalin", "reply", fmt.Sprint(asked.ID), "the second one; the first is dead code")
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
	h := twoRepos(t)
	m, err := h.store.For("lib").AddMail("galar", "someone/else", "not for you", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "dvalin", "reply", fmt.Sprint(m.ID), "answering somebody else's mail")
	if code == 0 || !strings.Contains(out, "in your mailbox") {
		t.Errorf("expected a refusal naming whose mailbox it is (%d): %s", code, out)
	}
}

// TestTheUserRepliesFromWhereTheyAreReading is the other half: the recipient comes from the row, so
// neither front-end asks who wrote it.
func TestTheUserRepliesFromWhereTheyAreReading(t *testing.T) {
	h, ps := noteSender(t, "dvalin")
	if out, code := execAs(t, h, "dvalin", "fyi", "the config field is documented backwards"); code != 0 {
		t.Fatalf("fyi failed (%d): %s", code, out)
	}
	note, _ := ps.UnreadMail(api.SenderUser)
	if len(note) != 1 {
		t.Fatalf("expected the note, got %d", len(note))
	}
	if err := h.ReplyToMail(note[0].ID, "fix it in the same PR then"); err != nil {
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
	if err := h.ReplyToMail(hubMsg.ID, "thanks"); err == nil {
		t.Error("replying to a hub notification should be refused for the user too")
	}
	_ = store.Agent{}
}

// TestAnAgentSeesTheIdsItCanReplyTo is the gap this rendering exposed: reading mail is the only place an
// agent learns an id, so the read half has to show them — otherwise `reply <mail-id>` is a verb whose
// argument the agent can never obtain. Both spellings are accepted, since the bare one is what anything
// already told to an agent contains.
func TestAnAgentSeesTheIdsItCanReplyTo(t *testing.T) {
	h := twoRepos(t)
	ps := h.store.For(testProject)
	m, err := ps.AddMail("dvalin", "lib/galar", "the second call is cached upstream", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "dvalin", "mail")
	if code != 0 {
		t.Fatalf("mail failed (%d): %s", code, out)
	}
	if !strings.Contains(out, api.MailID(m.ID)) {
		t.Errorf("reading mail should show each id, so a reply has something to name: %s", out)
	}

	// The prefixed form works...
	if out, code := execIn(t, h, testProject, "dvalin", "reply", api.MailID(m.ID), "understood"); code != 0 {
		t.Fatalf("a prefixed id should be accepted (%d): %s", code, out)
	}
	// ...and so does the bare one, which is what older messages and shell history carry.
	m2, err := ps.AddMail("dvalin", "lib/galar", "one more thing", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if out, code := execIn(t, h, testProject, "dvalin", "reply", fmt.Sprint(m2.ID), "also understood"); code != 0 {
		t.Fatalf("a bare id must keep working (%d): %s", code, out)
	}
	// And a refusal shows the shape rather than just saying no.
	out, code = execAs(t, h, "dvalin", "reply", "nonsense", "hello")
	if code == 0 || !strings.Contains(out, api.MailIDPrefix) {
		t.Errorf("the refusal should show what an id looks like (%d): %s", code, out)
	}
}
