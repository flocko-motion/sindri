package mail

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// twoRepos seeds an agent in each of two projects, which is the shape this verb exists for: the user
// tells a worker in one repo to explain something to an agent in another.
func twoRepos(t *testing.T) (*Box, *store.ProjectStore) {
	t.Helper()
	b, ps, _ := newBox(t)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if err := b.store.RegisterProject("lib", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := b.store.For("lib").PutAgent(store.Agent{Name: "galar", Role: "planner", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	return b, ps
}

// TestAnAgentMailsAnotherByBareNameAcrossRepos is the driving case: the sender was GIVEN the name, so a
// bare name resolves across the fleet with no roster listing needed — and the recipient sees where the
// message came from, since one arriving from another repo is useless without that.
func TestAnAgentMailsAnotherByBareNameAcrossRepos(t *testing.T) {
	b, ps := twoRepos(t)
	out, code := verbAs(t, b, ps, "dvalin", "mail", "galar", "the shelling out happens twice per sync")
	if code != 0 {
		t.Fatalf("mail failed (%d): %s", code, out)
	}
	got, err := b.store.For("lib").UnreadMail("galar")
	if err != nil || len(got) != 1 {
		t.Fatalf("the message should be waiting in the other repo, got %+v (err %v)", got, err)
	}
	// Attribution is QUALIFIED where the address was bare, and the reply path is the bare name again.
	if !strings.HasSuffix(got[0].Sender, "/dvalin") {
		t.Errorf("sender = %q, want it qualified by repo", got[0].Sender)
	}
	if got[0].Pushed {
		t.Error("agents mail each other, they do not wake each other")
	}
	if !strings.Contains(out, "next `sindri`") {
		t.Errorf("the reply should say when it will be read: %s", out)
	}
}

// TestAnAmbiguousNameIsRefusedWithBothCandidates: global uniqueness is the allocator's convention, not
// a schema constraint, and delivering to the wrong dvalin is the one failure worth engineering against.
// The refusal has to name what it collided with and the way through, or a safety check becomes a dead end.
func TestAnAmbiguousNameIsRefusedWithBothCandidates(t *testing.T) {
	b, ps := twoRepos(t)
	// The same name in both repos — reachable by hand, since the schema keys mailboxes (project, name)
	// and only the allocator keeps names unique.
	if err := b.store.For("lib").PutAgent(store.Agent{Name: "nori", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutAgent(store.Agent{Name: "nori", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	out, code := verbAs(t, b, ps, "dvalin", "mail", "nori", "which of you is it")
	if code == 0 {
		t.Errorf("an ambiguous recipient must be refused rather than picked: %s", out)
	}
	if !strings.Contains(out, "ambiguous") {
		t.Errorf("the refusal should say so plainly: %s", out)
	}
	if strings.Count(out, "/nori") < 2 {
		t.Errorf("it should name both candidates in full: %s", out)
	}
	if all, _ := b.store.AllMail(0); len(all) != 0 {
		t.Errorf("nothing should have been delivered: %+v", all)
	}

	// And the qualified form is the way through — never required, but it exists so ambiguity is not a
	// permanent wall.
	out, code = verbAs(t, b, ps, "dvalin", "mail", "lib/nori", "you, the one in lib")
	if code != 0 {
		t.Fatalf("the qualified form should deliver (%d): %s", code, out)
	}
	if got, _ := b.store.For("lib").UnreadMail("nori"); len(got) != 1 {
		t.Errorf("expected the lib nori to have it, got %d", len(got))
	}
	if got, _ := ps.UnreadMail("nori"); len(got) != 0 {
		t.Errorf("and the other one to have nothing, got %d", len(got))
	}
}

// TestAnUnknownRecipientIsRefused, since a durable message to nobody would hide the typo for ever.
func TestAnUnknownRecipientIsRefused(t *testing.T) {
	b, ps := twoRepos(t)
	out, code := verbAs(t, b, ps, "dvalin", "mail", "nobody", "hello")
	if code == 0 || !strings.Contains(out, "no agent named") {
		t.Errorf("expected a refusal naming the problem (%d): %s", code, out)
	}
}

// TestMailIsNeitherCappedNorBudgeted: mail carries whatever length it needs, whoever reads it. A
// length cap costs the messages worth sending — a rejection's feedback and a gate report do not fit
// in a few hundred characters — and mail is ASKED FOR, read when its reader chooses, where the note
// channel arrives unbidden and keeps its own cap for that reason (-> MaxNoteLen). The per-claim grant
// and the fleet ceiling stay off this channel too: those bound unprompted attention.
func TestMailIsNeitherCappedNorBudgeted(t *testing.T) {
	b, ps := twoRepos(t)
	long := strings.Repeat("x", MaxNoteLen*4)
	if out, code := verbAs(t, b, ps, "dvalin", "mail", "galar", long); code != 0 {
		t.Errorf("mail to an agent carries its length (%d): %s", code, out)
	}
	if all, _ := b.store.AllMail(0); len(all) != 1 || len([]rune(all[0].Body)) < MaxNoteLen {
		t.Errorf("it should be stored whole, never truncated: %+v", all)
	}
	// No grant is consulted: dvalin has claimed nothing, and can still mail an agent as often as it
	// has something to say.
	for i := 0; i < workflowNotesPerClaimPlusOne(); i++ {
		if out, code := verbAs(t, b, ps, "dvalin", "mail", "galar", "another thing worth knowing"); code != 0 {
			t.Fatalf("message %d should not be budgeted (%d): %s", i+1, code, out)
		}
	}
}

// TestAReplyToTheUserCarriesItsLength: the user is a recipient like any other on this channel. They
// asked for the message — a reply answers one they wrote — so the cost is theirs to have chosen, and
// capping it refused exactly the answers worth reading. The unprompted note keeps its cap instead.
func TestAReplyToTheUserCarriesItsLength(t *testing.T) {
	b, ps := twoRepos(t)
	if err := b.MailAgent(testProject, "dvalin", "what did the sync actually do?"); err != nil {
		t.Fatal(err)
	}
	waiting, err := ps.UnreadMail("dvalin")
	if err != nil || len(waiting) != 1 {
		t.Fatalf("the user's question should be waiting, got %+v (err %v)", waiting, err)
	}
	long := strings.Repeat("x", MaxNoteLen*4)
	if out, code := verbAs(t, b, ps, "dvalin", "reply", api.MailID(waiting[0].ID), long); code != 0 {
		t.Fatalf("a reply to the user carries its length (%d): %s", code, out)
	}
	answer, err := ps.UnreadMail(api.SenderUser)
	if err != nil || len(answer) != 1 {
		t.Fatalf("the answer should be in the user's mailbox, got %+v (err %v)", answer, err)
	}
	if len([]rune(answer[0].Body)) != len([]rune(long)) {
		t.Errorf("stored whole, never truncated: %d of %d characters", len([]rune(answer[0].Body)), len([]rune(long)))
	}
}

// workflowNotesPerClaimPlusOne is one more than the note grant, so the loop above proves the agent
// channel is not sharing that budget rather than merely staying under it.
func workflowNotesPerClaimPlusOne() int { return NotesPerClaim + 1 }

// TestMailingYourselfIsRefused: it is a sign the sender meant somebody else, and saying so is more
// useful than delivering it.
func TestMailingYourselfIsRefused(t *testing.T) {
	b, ps := twoRepos(t)
	out, code := verbAs(t, b, ps, "dvalin", "mail", "dvalin", "note to self")
	if code == 0 || !strings.Contains(out, "that is you") {
		t.Errorf("expected a refusal pointing at the log (%d): %s", code, out)
	}
}

// TestSendingDoesNotConsumeTheReadHalf: one verb, two halves — a bare `mail` must still read.
func TestSendingDoesNotConsumeTheReadHalf(t *testing.T) {
	b, ps := twoRepos(t)
	if _, err := ps.AddMail("dvalin", "hub", "[hub] a verdict", false, 0); err != nil {
		t.Fatal(err)
	}
	out, code := verbAs(t, b, ps, "dvalin", "mail")
	if code != 0 || !strings.Contains(out, "a verdict") {
		t.Errorf("bare `mail` should still hand over what is waiting (%d): %s", code, out)
	}
}
