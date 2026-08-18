package workflow

import (
	"path/filepath"
	"testing"

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
	id, woken := e.NudgeMailWaiting("proj", "dvalin", 0)
	if !woken {
		t.Fatal("an idle agent with unread mail should be woken")
	}
	if id == 0 {
		t.Error("the nudge should report what it nudged for, so it is not repeated")
	}
	if len(deps.delivered) != 1 || deps.delivered[0].Mail || !deps.delivered[0].Push {
		t.Errorf("the wake is push-only — what must be read is already kept: %+v", deps.delivered)
	}
	if len(deps.injectedText) != 1 || !contains(deps.injectedText[0], "sindri mail") {
		t.Errorf("it should name the verb that reads it: %q", deps.injectedText)
	}
}

// TestTheSameMessageIsNotNudgedTwice: an agent told once and still not reading is either choosing not
// to or is wedged, and repeating it every tick burns its context and teaches it to skim.
func TestTheSameMessageIsNotNudgedTwice(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleAgentWithMail(t, deps)
	id, _ := e.NudgeMailWaiting("proj", "dvalin", 0)
	if _, woken := e.NudgeMailWaiting("proj", "dvalin", id); woken {
		t.Error("the same waiting message must not be nudged for twice")
	}
	// NEW mail is a new thing waiting, so it earns a second wake.
	if _, err := ps.AddMail("dvalin", "galar", "and the config is documented backwards", false, 0); err != nil {
		t.Fatal(err)
	}
	if _, woken := e.NudgeMailWaiting("proj", "dvalin", id); !woken {
		t.Error("mail that arrived after the last nudge should wake it again")
	}
}

// TestAnAgentThatNeedsAHumanIsNotWoken: it cannot act on mail, and a nudge it cannot answer is noise on
// the one signal the user relies on to spot a stuck agent.
func TestAnAgentThatNeedsAHumanIsNotWoken(t *testing.T) {
	// busy covers every runtime that is not an empty prompt — mid-turn, blocked, signed out, cut off.
	deps := &stubDeps{busy: map[string]bool{"dvalin": true}}
	e, _ := idleAgentWithMail(t, deps)
	if _, woken := e.NudgeMailWaiting("proj", "dvalin", 0); woken {
		t.Error("an agent that is not at an empty prompt must not be nudged")
	}
	if len(deps.delivered) != 0 {
		t.Errorf("nothing should have been sent: %+v", deps.delivered)
	}
}

// TestAParkedAgentIsNotWoken: retired or context-full is a state the hub itself put the agent in, and
// told it to wait in. The stall nudge exempts it for the same reason.
func TestAParkedAgentIsNotWoken(t *testing.T) {
	deps := &stubDeps{ctxTokens: 900_000, ctxWindow: 1_000_000, ctxOK: true}
	e, _ := idleAgentWithMail(t, deps)
	if _, woken := e.NudgeMailWaiting("proj", "dvalin", 0); woken {
		t.Error("a context-full agent was woken for mail it cannot act on")
	}
}

// TestAnAgentHoldingWorkIsNotWoken: it is going to call `sindri` anyway, and the directive hands it its
// mail before anything else — so a wake would be redundant noise mid-task.
func TestAnAgentHoldingWorkIsNotWoken(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleAgentWithMail(t, deps)
	if err := ps.SetState(store.AgentState{Agent: "dvalin", Task: "sd-1", Branch: "sd-1", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	if _, woken := e.NudgeMailWaiting("proj", "dvalin", 0); woken {
		t.Error("an agent holding work reads its mail at its next ask, without being prodded")
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
	if _, woken := e.NudgeMailWaiting("proj", "dvalin", 0); woken {
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
