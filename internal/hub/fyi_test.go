package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// noteSender seeds a worker holding a claim, with its note grant given as a claim gives it.
func noteSender(t *testing.T, name string) (*Hub, *store.ProjectStore) {
	t.Helper()
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: name, Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: name, Task: "td-1", Branch: "td-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.GrantNotes(name, workflow.NotesPerClaim); err != nil {
		t.Fatal(err)
	}
	return h, ps
}

// TestANoteReachesTheUsersMailboxAndSaysWhatIsLeft: the user is a recipient like any agent, addressed
// by the same spelling that stamps a message FROM them — and the reply states the remainder, because
// known scarcity selects better than a cap discovered by hitting it.
func TestANoteReachesTheUsersMailboxAndSaysWhatIsLeft(t *testing.T) {
	h, ps := noteSender(t, "dvalin")
	out, code := execAs(t, h, "dvalin", "fyi", "the td adapter shells out twice per task; nobody owns that")
	if code != 0 {
		t.Fatalf("fyi failed (%d): %s", code, out)
	}
	mail, err := h.store.AllMail(0)
	if err != nil || len(mail) != 1 {
		t.Fatalf("the note should be in a mailbox, got %+v (err %v)", mail, err)
	}
	if mail[0].Agent != api.SenderUser {
		t.Errorf("recipient = %q, want the user", mail[0].Agent)
	}
	if mail[0].Sender != "dvalin" {
		t.Errorf("sender = %q, want the agent that noticed it", mail[0].Sender)
	}
	if mail[0].Pushed {
		t.Error("the user has no session to push into, so nothing should claim a wake landed")
	}
	if !strings.Contains(out, "one note left") {
		t.Errorf("the reply should say what is left: %s", out)
	}
	if n, _ := ps.NotesLeft("dvalin"); n != workflow.NotesPerClaim-1 {
		t.Errorf("notes left = %d, want %d", n, workflow.NotesPerClaim-1)
	}
}

// TestTheGrantIsSpentThenRefusedWithSomewhereElseToGo: the third note is refused, and the refusal
// names where the material actually belongs — a refusal that only says no leaves the agent to guess,
// and guessing here means sending it again.
func TestTheGrantIsSpentThenRefusedWithSomewhereElseToGo(t *testing.T) {
	h, _ := noteSender(t, "dvalin")
	for i := 0; i < workflow.NotesPerClaim; i++ {
		if out, code := execAs(t, h, "dvalin", "fyi", "something", "worth", "knowing"); code != 0 {
			t.Fatalf("note %d refused early (%d): %s", i+1, code, out)
		}
	}
	out, code := execAs(t, h, "dvalin", "fyi", "one", "more", "thing")
	if code == 0 {
		t.Errorf("the third note must be refused: %s", out)
	}
	for _, want := range []string{"sindri comment", "sindri escalate", "PR body"} {
		if !strings.Contains(out, want) {
			t.Errorf("the refusal should name %q as the home instead: %s", want, out)
		}
	}
	if mail, _ := h.store.AllMail(0); len(mail) != workflow.NotesPerClaim {
		t.Errorf("a refused note must not be stored: %d rows", len(mail))
	}
}

// TestTheGrantReplacesRatherThanAccumulates is what stops any quota becoming an occasional flood:
// finishing a claim with everything unspent must not start the next one at double.
func TestTheGrantReplacesRatherThanAccumulates(t *testing.T) {
	_, ps := noteSender(t, "dvalin")
	if err := ps.GrantNotes("dvalin", workflow.NotesPerClaim); err != nil { // a second claim, nothing spent
		t.Fatal(err)
	}
	if n, _ := ps.NotesLeft("dvalin"); n != workflow.NotesPerClaim {
		t.Errorf("notes left = %d after a fresh grant, want %d — banking is what floods", n, workflow.NotesPerClaim)
	}
}

// TestAnOverLongNoteIsRefusedNotTruncated: a silent cut drops the point and teaches nothing, so the
// next note is exactly as long. The refusal must also forbid the obvious workaround.
func TestAnOverLongNoteIsRefusedNotTruncated(t *testing.T) {
	h, ps := noteSender(t, "dvalin")
	long := strings.Repeat("x", maxNoteLen+1)
	out, code := execAs(t, h, "dvalin", "fyi", long)
	if code == 0 {
		t.Errorf("an over-length note must be refused: %s", out)
	}
	if mail, _ := h.store.AllMail(0); len(mail) != 0 {
		t.Errorf("nothing should be stored, least of all a truncated version: %+v", mail)
	}
	if !strings.Contains(out, "CUT IT") || !strings.Contains(out, "do NOT split") {
		t.Errorf("the refusal should say to cut it and not to split it: %s", out)
	}
	// And it costs nothing: the agent still has its whole grant to spend on a shorter note.
	if n, _ := ps.NotesLeft("dvalin"); n != workflow.NotesPerClaim {
		t.Errorf("a refusal should not spend the grant, got %d left", n)
	}
}

// TestTheFleetCeilingProtectsTheUserFromImpeccableAgents is the limit that matters: a work-based grant
// scales with the fleet and one person's attention does not, so forty agents each behaving correctly
// still bury them. It refuses rather than queueing, and every refusal is logged as the only evidence
// for what the ceiling should be.
func TestTheFleetCeilingProtectsTheUserFromImpeccableAgents(t *testing.T) {
	h, ps := noteSender(t, "nori")
	// The hour's ceiling is already spent, one note each — every one of those agents within its grant.
	for i := 0; i < fleetNotesPerHour; i++ {
		if _, err := ps.AddMail(api.SenderUser, "someone", "a perfectly good note", false); err != nil {
			t.Fatal(err)
		}
	}
	out, code := execAs(t, h, "nori", "fyi", "mine", "is", "good", "too")
	if code == 0 {
		t.Errorf("the ceiling must refuse an otherwise-correct note: %s", out)
	}
	if !strings.Contains(out, "refused rather than queued") {
		t.Errorf("the refusal should be honest that nothing is held: %s", out)
	}
	// Logged, with the count — this is the number that says whether the ceiling is right.
	evs, _ := ps.Events("nori", 20)
	found := false
	for _, e := range evs {
		if e.Type == "fyi-refused" && strings.Contains(e.Payload, "fleet ceiling") {
			found = true
		}
	}
	if !found {
		t.Errorf("every refusal must be logged: %v", evs)
	}
}

// TestNoAgentMayBeCalledUser: the human's mailbox is addressed by that name, so an agent holding it
// would share a mailbox with the person it reports to, with nothing in a row to tell them apart.
func TestNoAgentMayBeCalledUser(t *testing.T) {
	h := newHub(t)
	if _, err := h.NewAgent(testProject, api.SenderUser, "worker", ""); err == nil {
		t.Error(`an agent named "user" must be refused — it is the reserved recipient`)
	}
}

// TestAnAgentThatHasClaimedNothingHasNoNotes: the budget fails CLOSED. An agent with no state row has
// never been anywhere and looked at anything, and the grant is what ties the right to speak to having
// done so — so a missing row must read as nothing granted rather than as an unlimited purse.
func TestAnAgentThatHasClaimedNothingHasNoNotes(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "fresh", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if n, err := ps.NotesLeft("fresh"); err != nil || n != 0 {
		t.Errorf("notes left = %d (err %v), want 0 for an agent that has claimed nothing", n, err)
	}
	if out, code := execAs(t, h, "fresh", "fyi", "something"); code == 0 {
		t.Errorf("it should be refused: %s", out)
	}
}
