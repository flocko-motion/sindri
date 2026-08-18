package hub

import (
	"strings"
	"testing"
	"time"

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
		if _, err := ps.AddMail(api.SenderUser, "someone", "a perfectly good note", false, 0); err != nil {
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

// TestRepliesDoNotSpendTheFleetCeiling is the mirror of the test above, and the seam where two of this
// feature's parts disagreed: the ceiling bounds what the user must read WITHOUT HAVING ASKED, so an
// answer to something they sent cannot count against it. The user mailing six agents a question and
// getting six answers must not close the channel on the whole fleet for an hour — and the refusal it
// would produce is false twice over, since nobody sent a note and the six were what was asked for.
func TestRepliesDoNotSpendTheFleetCeiling(t *testing.T) {
	h, ps := noteSender(t, "nori")
	// A whole ceiling's worth of REPLIES: the user asked, and this many agents answered.
	for i := 0; i < fleetNotesPerHour+2; i++ {
		asked, err := ps.AddMail("nori", api.SenderUser, "what is holding this up?", false, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ps.AddMail(api.SenderUser, "nori", "the td adapter, still", false, asked.ID); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := h.store.NotesToUserSince(time.Now().Add(-fleetNoteWindow)); err != nil || n != 0 {
		t.Errorf("unprompted notes = %d (err %v), want 0 — every row in the window is a reply", n, err)
	}
	if out, code := execAs(t, h, "nori", "fyi", "and the pod's go lags go.mod"); code != 0 {
		t.Errorf("a note must still go through (%d): %s", code, out)
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

// TestAReviewerGetsAGrantWhenGivenAReview is the role the reviewer of this change named: it reads whole
// diffs across subsystems that are nobody's task, so it is the most likely to notice something with no
// other home — and it reaches neither claimLeaf nor startSubtask, so nothing else would grant it.
func TestAReviewerGetsAGrantWhenGivenAReview(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "rev", Role: "reviewer", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-td-1", Task: "td-1", Agent: "dvalin", Branch: "td-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	// Through the reviewer's OWN pickup — asking the hub for work claims an unclaimed review. The other
	// route (RequestReview picking a free reviewer) needs a live pod, which a test hub has not; both go
	// through assignReview, which is where the grant is.
	if _, err := ps.AddReview("pr-td-1", "check it"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.wf.AgentDirective(t.Context(), testProject, "rev"); err != nil {
		t.Fatalf("AgentDirective for the reviewer: %v", err)
	}
	if n, _ := ps.NotesLeft("rev"); n != workflow.NotesPerClaim {
		t.Fatalf("a reviewer given a review should hold the grant, got %d", n)
	}
	out, code := execAs(t, h, "rev", "fyi", "the adapter shells out twice per sync; no task owns that")
	if code != 0 {
		t.Fatalf("a reviewer must be able to send (%d): %s", code, out)
	}
}

// TestTheCoauthorIsNotOfferedAVerbItCannotUse: it works beside the user in a shared terminal, so it
// already has their attention — and a verb advertised to a role that can never use it is worse than
// one absent, because the refusal has to invent a reason.
func TestTheCoauthorIsNotOfferedAVerbItCannotUse(t *testing.T) {
	h := newHub(t)
	if err := h.store.For(testProject).PutAgent(store.Agent{Name: "hepti", Role: "coauthor", Workspace: "."}); err != nil {
		t.Fatal(err)
	}
	cmds, err := h.AgentCommands(testProject, "hepti")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cmds {
		if c.Name == "fyi" {
			t.Error("a coauthor should not be offered fyi — it is in the room with the user")
		}
	}
	// And it reads as unknown rather than as blocked, which is what role isolation means here.
	out, code := execAs(t, h, "hepti", "fyi", "something")
	if code != 127 || !strings.Contains(out, "unknown") {
		t.Errorf("expected an unknown verb for a coauthor (code %d): %s", code, out)
	}
}

// TestTheRefusalIsTrueBeforeAnyClaimToo: the same words serve a grant that was spent and one never
// given, so nothing promises a "next claim" that has not happened, and nothing hardcodes "both".
func TestTheRefusalIsTrueBeforeAnyClaimToo(t *testing.T) {
	h := newHub(t)
	if err := h.store.For(testProject).PutAgent(store.Agent{Name: "fresh", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	out, _ := execAs(t, h, "fresh", "fyi", "something")
	for _, bad := range []string{"used both", "next claim starts"} {
		if strings.Contains(out, bad) {
			t.Errorf("the refusal says %q, which is false for an agent that has never claimed: %s", bad, out)
		}
	}
	if !strings.Contains(out, "no notes left on this claim") {
		t.Errorf("it should say what is true of both cases: %s", out)
	}
}
