package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAUsersMailWaitsAndDoesNotPush is the whole of what makes it a second action rather than a mode
// of `tell`: choosing mail IS the choice not to interrupt, so it must not also be injected — and it
// must reach an agent that tell cannot, which is any agent with no live session.
func TestAUsersMailWaitsAndDoesNotPush(t *testing.T) {
	h, ps := mailAgent(t)
	if err := h.MailAgent(testProject, "dvalin", "when you get to it, note that the base moved"); err != nil {
		t.Fatalf("MailAgent: %v", err)
	}
	unread, err := ps.UnreadMail("dvalin")
	if err != nil || len(unread) != 1 {
		t.Fatalf("the message must be waiting, got %+v (err %v)", unread, err)
	}
	if unread[0].Pushed {
		t.Error("a user's mail must not be pushed — not interrupting is the reason to pick it")
	}
	// Stamped as the user's, like `tell`, so the agent weights it as a human instruction and the Mail
	// view attributes it to a person rather than to the hub.
	if unread[0].Sender != "user" {
		t.Errorf("sender = %q, want user", unread[0].Sender)
	}
	if !strings.HasPrefix(unread[0].Body, "[user] ") {
		t.Errorf("the body should carry the provenance tag the agent reads: %q", unread[0].Body)
	}
	// And the agent is told at its next ask, which is what "it will be read" means in practice.
	dir, err := h.wf.AgentDirective(t.Context(), testProject, "dvalin")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dir, "unread") {
		t.Errorf("the directive should announce it: %q", dir)
	}
}

// TestMailingAnUnknownAgentIsRefused: a name with no roster entry is a typo, and mail is durable —
// silently keeping a message for an agent that does not exist would hide the mistake for ever.
func TestMailingAnUnknownAgentIsRefused(t *testing.T) {
	h, _ := mailAgent(t)
	if err := h.MailAgent(testProject, "nobody", "hello"); err == nil {
		t.Error("mailing an agent that does not exist should be refused")
	}
	if all, _ := h.store.AllMail(0); len(all) != 0 {
		t.Errorf("nothing should have been stored: %+v", all)
	}
}

// TestMailReachesASignedOutAgent is the case sd-f66d3b's refusal left behind: text typed at a /login
// prompt vanishes, so `tell` asks about it — but mail never touches the session, so it simply works
// and is read when the session recovers. The guard belongs to the push path alone.
func TestMailReachesASignedOutAgent(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "gloin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	// No pod at all, which is the strongest form of "cannot be typed into".
	if err := h.MailAgent(testProject, "gloin", "the token is renewed, carry on"); err != nil {
		t.Fatalf("mail must not depend on a live session: %v", err)
	}
	if n, _ := ps.UnreadMailCount("gloin"); n != 1 {
		t.Errorf("unread = %d, want the message waiting", n)
	}
}
