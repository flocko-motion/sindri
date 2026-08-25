package workflow

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// idleAgentWithMail seeds a live agent sitting at an empty prompt with one unread message — the exact
// shape this feature exists for: an agent that finished, stopped asking, and has something waiting.
func idleAgentWithMail(t *testing.T, deps *stubDeps) (*Engine, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	deps.root, deps.alive = root, true
	if err := st.RegisterProject("proj", root); err != nil {
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
	deps := &stubDeps{}
	e, _ := idleAgentWithMail(t, deps)
	if !e.NudgeMailWaiting("proj", "dvalin") {
		t.Fatal("an idle agent with unread mail should be woken")
	}
	if len(deps.delivered) != 1 || deps.delivered[0].Mail || !deps.delivered[0].Push {
		t.Errorf("the wake is push-only — what must be read is already kept: %+v", deps.delivered)
	}
	if len(deps.injectedText) != 1 || !contains(deps.injectedText[0], "Run `sindri`") {
		t.Errorf("it should point at the plain verb, which now delivers mail and the directive together: %q", deps.injectedText)
	}
}

// TestTheSameMessageIsNotNudgedTwice: an agent told once and still not reading is either choosing not
// to or is wedged, and repeating it every tick burns its context and teaches it to skim. Nothing is
// carried between the calls — what the agent has been told is on the MESSAGE, which is what makes this
// hold across a hub restart too.
func TestTheSameMessageIsNotNudgedTwice(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleAgentWithMail(t, deps)
	e.NudgeMailWaiting("proj", "dvalin")
	if e.NudgeMailWaiting("proj", "dvalin") {
		t.Error("the same waiting message must not be nudged for twice")
	}
	// NEW mail is a new thing waiting, so it earns a second wake.
	if _, err := ps.AddMail("dvalin", "galar", "and the config is documented backwards", false, 0); err != nil {
		t.Fatal(err)
	}
	if !e.NudgeMailWaiting("proj", "dvalin") {
		t.Error("mail that arrived after the last nudge should wake it again")
	}
}

// TestOneNudgeCoversTheWholeMailbox: eleven messages must be one interruption, not eleven — and the
// count it states is the whole unread total, since that is what the agent has to deal with.
func TestOneNudgeCoversTheWholeMailbox(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleAgentWithMail(t, deps)
	for i := 0; i < 10; i++ {
		if _, err := ps.AddMail("dvalin", "hub", "another one", false, 0); err != nil {
			t.Fatal(err)
		}
	}
	if !e.NudgeMailWaiting("proj", "dvalin") {
		t.Fatal("eleven unread messages should be worth a wake")
	}
	if len(deps.injectedText) != 1 || !contains(deps.injectedText[0], "11 unread") {
		t.Errorf("one wake, naming the whole unread count: %q", deps.injectedText)
	}
	// Everything waiting was marked, so the next tick is silent — announcing eight of eleven and marking
	// all eleven would lose three for ever, and the reverse would repeat them.
	if e.NudgeMailWaiting("proj", "dvalin") {
		t.Error("one nudge should have covered every message then waiting")
	}
}

// TestAnUndeliveredNudgeLeavesTheMailUnannounced: marking before the push lands would lose the
// announcement for a message nobody was ever told about — the agent would sit on mail in silence.
func TestAnUndeliveredNudgeLeavesTheMailUnannounced(t *testing.T) {
	deps := &stubDeps{deliverErr: true}
	e, ps := idleAgentWithMail(t, deps)
	if e.NudgeMailWaiting("proj", "dvalin") {
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
			deps := &stubDeps{}
			e, ps := idleAgentWithMail(t, deps)
			if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: role, Workspace: ".worktrees/dvalin"}); err != nil {
				t.Fatal(err)
			}
			if !e.NudgeMailWaiting("proj", "dvalin") {
				t.Errorf("a %s with unread mail must be woken too", role)
			}
		})
	}
}

// TestAnAgentThatNeedsAHumanIsNotWoken: it cannot act on mail, and a nudge it cannot answer is noise on
// the one signal the user relies on to spot a stuck agent.
func TestAnAgentThatNeedsAHumanIsNotWoken(t *testing.T) {
	// busy covers every runtime that is not an empty prompt — mid-turn, blocked, signed out, cut off.
	deps := &stubDeps{busy: map[string]bool{"dvalin": true}}
	e, _ := idleAgentWithMail(t, deps)
	if e.NudgeMailWaiting("proj", "dvalin") {
		t.Error("an agent that is not at an empty prompt must not be nudged")
	}
	if len(deps.delivered) != 0 {
		t.Errorf("nothing should have been sent: %+v", deps.delivered)
	}
}

// TestARetiredAgentHoldingWorkIsWoken: retiring an agent promises it FINISHES what it holds, so a
// gate result or a rejection about that work is exactly what it still needs. Read as silence, this
// left a retired dain sitting on an unread gate failure for its own held task, unable to learn of it.
func TestARetiredAgentHoldingWorkIsWoken(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleAgentWithMail(t, deps)
	a, _, err := ps.GetAgent("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if !e.NudgeMailWaiting("proj", "dvalin") {
		t.Error("a retired agent holding work went untold; it cannot finish what it holds without the news")
	}
}

// TestARetiredAgentHoldingNothingIsLeftAlone is the other half: with nothing in hand it is done,
// which is all retirement ever meant, so no mail can be about work it still owes.
func TestARetiredAgentHoldingNothingIsLeftAlone(t *testing.T) {
	deps := &stubDeps{holdsNothing: true}
	e, ps := idleAgentWithMail(t, deps)
	a, _, err := ps.GetAgent("dvalin")
	if err != nil {
		t.Fatal(err)
	}
	a.Retired = true
	if err := ps.PutAgent(a); err != nil {
		t.Fatal(err)
	}
	if e.NudgeMailWaiting("proj", "dvalin") {
		t.Error("a retired agent with nothing in hand was woken; it has already finished")
	}
}

// TestAnAgentHoldingWorkIsWoken inverts what this asserted: that an agent mid-task "is going to call
// `sindri` anyway". Twice it was not — it believed a gate result was still coming, so it waited on a
// verdict already sitting unread in its own mailbox. Mail is most urgent while work is held, since
// that is what a verdict, a rejection or a cancellation is about.
func TestAnAgentHoldingWorkIsWoken(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleAgentWithMail(t, deps)
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "sd-1", Branch: "sd-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if !e.NudgeMailWaiting("proj", "dvalin") {
		t.Error("an agent holding work was left unaware of mail about that very work")
	}
}

// TestNothingWaitingIsNotAWake, the control: without it a change that nudged unconditionally would pass
// every test above.
func TestNothingWaitingIsNotAWake(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleAgentWithMail(t, deps)
	mail, _ := ps.UnreadMail("dvalin")
	for _, m := range mail {
		if err := ps.MarkMailRead(m.ID); err != nil {
			t.Fatal(err)
		}
	}
	if e.NudgeMailWaiting("proj", "dvalin") {
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
