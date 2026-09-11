package mail

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// idleAgentWithMail seeds a live agent sitting at an empty prompt with one unread message — the exact
// shape this feature exists for: an agent that finished, stopped asking, and has something waiting.
func idleAgentWithMail(t *testing.T, deps *stubDeps) (*Box, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	deps.up = true
	if err := st.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddMail("dvalin", "nori", "the adapter shells out twice", false, 0); err != nil {
		t.Fatal(err)
	}
	return New(st, deps), ps
}

// TestAnIdleAgentWithMailIsWoken is the guarantee: mail reaches an agent whenever it next asks the hub
// for work, and an agent that has stopped asking never does — so the hub asks it to look.
func TestAnIdleAgentWithMailIsWoken(t *testing.T) {
	deps := &stubDeps{up: true}
	b, _ := idleAgentWithMail(t, deps)
	if !b.NudgeMailWaiting("proj", "dvalin") {
		t.Fatal("an idle agent with unread mail should be woken")
	}
	// Push-only, observably: the notice went through Push and left no second row behind it. What
	// must be READ is the message already waiting; this only says that it is.
	if len(deps.pushed) != 1 {
		t.Errorf("the wake is one push: %+v", deps.pushed)
	}
	if all, _ := b.store.AllMail(0); len(all) != 1 {
		t.Errorf("the notice must not mailbox itself — the message it announces is already there: %+v", all)
	}
	if len(deps.texts) != 1 || !contains(deps.texts[0], "Run `sindri`") {
		t.Errorf("it should point at the plain verb, which now delivers mail and the directive together: %q", deps.texts)
	}
}

// TestTheSameMessageIsNotNudgedTwice: an agent told once and still not reading is either choosing not
// to or is wedged, and repeating it every tick burns its context and teaches it to skim. Nothing is
// carried between the calls — what the agent has been told is on the MESSAGE, which is what makes this
// hold across a hub restart too.
func TestTheSameMessageIsNotNudgedTwice(t *testing.T) {
	deps := &stubDeps{up: true}
	b, ps := idleAgentWithMail(t, deps)
	b.NudgeMailWaiting("proj", "dvalin")
	if b.NudgeMailWaiting("proj", "dvalin") {
		t.Error("the same waiting message must not be nudged for twice")
	}
	// NEW mail is a new thing waiting, so it earns a second wake.
	if _, err := ps.AddMail("dvalin", "galar", "and the config is documented backwards", false, 0); err != nil {
		t.Fatal(err)
	}
	if !b.NudgeMailWaiting("proj", "dvalin") {
		t.Error("mail that arrived after the last nudge should wake it again")
	}
}

// TestOneNudgeCoversTheWholeMailbox: eleven messages must be one interruption, not eleven — and the
// count it states is the whole unread total, since that is what the agent has to deal with.
func TestOneNudgeCoversTheWholeMailbox(t *testing.T) {
	deps := &stubDeps{up: true}
	b, ps := idleAgentWithMail(t, deps)
	for i := 0; i < 10; i++ {
		if _, err := ps.AddMail("dvalin", "hub", "another one", false, 0); err != nil {
			t.Fatal(err)
		}
	}
	if !b.NudgeMailWaiting("proj", "dvalin") {
		t.Fatal("eleven unread messages should be worth a wake")
	}
	if len(deps.texts) != 1 || !contains(deps.texts[0], "11 unread") {
		t.Errorf("one wake, naming the whole unread count: %q", deps.texts)
	}
	// Everything waiting was marked, so the next tick is silent — announcing eight of eleven and marking
	// all eleven would lose three for ever, and the reverse would repeat them.
	if b.NudgeMailWaiting("proj", "dvalin") {
		t.Error("one nudge should have covered every message then waiting")
	}
}

// TestAnUndeliveredNudgeLeavesTheMailUnannounced: marking before the push lands would lose the
// announcement for a message nobody was ever told about — the agent would sit on mail in silence.
func TestAnUndeliveredNudgeLeavesTheMailUnannounced(t *testing.T) {
	deps := &stubDeps{up: true, pushFails: true}
	b, ps := idleAgentWithMail(t, deps)
	if b.NudgeMailWaiting("proj", "dvalin") {
		t.Error("a wake that did not land is not a wake")
	}
	if unannounced, _, err := ps.UnannouncedMail("dvalin", time.Now()); err != nil || unannounced != 1 {
		t.Errorf("unannounced = %d (err %v), want the message still owed a wake", unannounced, err)
	}
}

// TestEveryRoleIsWoken is the hole this replaced: the only unasked push was role-gated to workers, and a
// worker is the role that needs it LEAST — it calls `sindri` constantly. A planner mid-conversation and a
// reviewer between verdicts can go hours without asking, which is how an agent came to sit on 11 unread.
func TestEveryRoleIsWoken(t *testing.T) {
	for _, role := range []string{"worker", "planner", "reviewer", "coauthor"} {
		t.Run(role, func(t *testing.T) {
			deps := &stubDeps{up: true}
			b, ps := idleAgentWithMail(t, deps)
			if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: role, Workspace: ".worktrees/dvalin"}); err != nil {
				t.Fatal(err)
			}
			if !b.NudgeMailWaiting("proj", "dvalin") {
				t.Errorf("a %s with unread mail must be woken too", role)
			}
		})
	}
}

// TestAnAgentAwayFromThePromptIsStillTold covers every runtime that is not an empty prompt — mid-turn,
// blocked, cut off. Each is told, and the blocked one most of all: an agent waiting on a human is
// exactly the one whose unread mail may BE the answer it is waiting for.
//
// What is sent is one line saying mail is waiting, so it costs a queued keystroke and nothing else.
// The agent still chooses when to read, which it cannot do while nobody has told it.
func TestAnAgentAwayFromThePromptIsStillTold(t *testing.T) {
	deps := &stubDeps{up: true}
	b, _ := idleAgentWithMail(t, deps)
	if !b.NudgeMailWaiting("proj", "dvalin") {
		t.Error("an agent away from the prompt was left untold; by the time it returns the news is stale")
	}
	if len(deps.pushed) != 1 {
		t.Errorf("the notice is one push: %+v", deps.pushed)
	}
	if all, _ := b.store.AllMail(0); len(all) != 1 {
		t.Errorf("the notice is push-only — the message itself is already in the mailbox: %+v", all)
	}
}

// TestAnAgentTheHubParkedIsLeftAlone: the mailbox asks whether a wake is welcome and honours the
// answer — it does not re-derive it. WHICH agents are parked (retired and holding nothing, a feature
// waiting on a verdict) is one rule, and it lives where it is decided
// (-> situation.Situation.ParkedByTheHub, tested there).
func TestAnAgentTheHubParkedIsLeftAlone(t *testing.T) {
	deps := &stubDeps{up: true, noWake: true}
	b, _ := idleAgentWithMail(t, deps)
	if b.NudgeMailWaiting("proj", "dvalin") {
		t.Error("an agent the hub told to wait was woken anyway — the notice complains about a state the hub chose")
	}
	if len(deps.pushed) != 0 {
		t.Errorf("nothing should have been typed at it: %v", deps.pushed)
	}
}

// TestAnAgentHoldingWorkIsWoken inverts what this asserted: that an agent mid-task "is going to call
// `sindri` anyway". Twice it was not — it believed a gate result was still coming, so it waited on a
// verdict already sitting unread in its own mailbox. Mail is most urgent while work is held, since
// that is what a verdict, a rejection or a cancellation is about.
func TestAnAgentHoldingWorkIsWoken(t *testing.T) {
	deps := &stubDeps{up: true}
	b, ps := idleAgentWithMail(t, deps)
	place(t, ps, "dvalin", "sd-1")
	if !b.NudgeMailWaiting("proj", "dvalin") {
		t.Error("an agent holding work was left unaware of mail about that very work")
	}
}

// TestNothingWaitingIsNotAWake, the control: without it a change that nudged unconditionally would pass
// every test above.
func TestNothingWaitingIsNotAWake(t *testing.T) {
	deps := &stubDeps{up: true}
	b, ps := idleAgentWithMail(t, deps)
	mail, _ := ps.UnreadMail("dvalin")
	for _, m := range mail {
		if err := ps.MarkMailRead(m.ID); err != nil {
			t.Fatal(err)
		}
	}
	if b.NudgeMailWaiting("proj", "dvalin") {
		t.Error("an empty mailbox is nothing to wake anyone for")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
