package fleet

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// addUnreadMail seeds one unread message for an agent and returns its id.
func addUnreadMail(t *testing.T, ps *store.ProjectStore, agent string) int64 {
	t.Helper()
	m, err := ps.AddMail(agent, "hub", "[hub] something happened", false, 0)
	if err != nil {
		t.Fatalf("AddMail: %v", err)
	}
	return m.ID
}

// TestUnreadMailIsDeliveredBeforeAnythingElse is the invariant that replaced eight tests of a
// deferral mechanism. Those deferred mail PAST a clear so it would not be marked read into a session
// about to be discarded. Unread mail means the agent is NOT DONE, and only a done agent is prepared
// for new work — so the hub PUTS the mail in the session and the agent is done, rather than holding
// it out of the workflow until it asks. It is idle at an empty prompt; the ask was never coming.
func TestUnreadMailIsDeliveredBeforeAnythingElse(t *testing.T) {
	deps := &stubDeps{CtxTokens: 900_000, CtxWindow: 1_000_000, CtxOK: true}
	e, ps := idleWorkerWithOpenTask(t, deps)
	addUnreadMail(t, ps, "dvalin")

	e.Look("repo", "dvalin")

	said := strings.Join(deps.Said(), "\n")
	if !strings.Contains(said, "something happened") {
		t.Fatalf("the mail must be put INTO the session, not waited for: %q", said)
	}
	if n, _ := ps.UnreadMailCount("dvalin"); n != 0 {
		t.Errorf("delivering the mail marks it read, got %d unread", n)
	}
	// The whole point of holding the agent: a message it never saw must not be thrown away by the
	// reset that prepares it for whatever comes next.
	if i, j := strings.Index(said, "something happened"), len(deps.Cleared); i < 0 && j > 0 {
		t.Errorf("a clear fired without the mail ever landing: %v", deps.Cleared)
	}
}

// TestReadingTheMailReleasesTheAgent is the other half: the mailbox emptying is the exit, and the
// ordinary flow resumes at once rather than at some later beat.
func TestReadingTheMailReleasesTheAgent(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	id := addUnreadMail(t, ps, "dvalin")
	e.Look("repo", "dvalin")

	if err := ps.MarkMailRead(id); err != nil {
		t.Fatalf("MarkMailRead: %v", err)
	}
	e.Look("repo", "dvalin")

	if st, _ := ps.GetState("dvalin"); st.Task == "" {
		t.Errorf("once the mailbox is empty the agent is prepared and handed work, got %+v", st)
	}
}

// TestTheMailReachesAnAgentThatNeverAsks is the deadlock in one test: nothing here calls the hub on
// the agent's behalf, and the mail still arrives. Reading was the only way out of not-done and only
// an ask could do the reading, so an agent sitting at a prompt waited for a hub that waited for it.
func TestTheMailReachesAnAgentThatNeverAsks(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	addUnreadMail(t, ps, "dvalin")

	e.Look("repo", "dvalin")

	if !strings.Contains(strings.Join(deps.Said(), "\n"), "something happened") {
		t.Errorf("the message never reached the session: %v", deps.Said())
	}
	if n, _ := ps.UnreadMailCount("dvalin"); n != 0 {
		t.Errorf("the mail is read once delivered, got %d unread", n)
	}
}

// TestTheAgentIsLeftToReadWhatItWasJustHanded: delivering the mail STARTS a turn, and that turn is
// the whole point of holding the agent here. Moving on the moment the message lands would hand it
// work and clear its session out from under the message it is reading — the loss the rule exists to
// prevent, reintroduced by the delivery meant to honour it.
func TestTheAgentIsLeftToReadWhatItWasJustHanded(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	deps.Turning("dvalin", true) // mid-turn: reacting to what just landed
	addUnreadMail(t, ps, "dvalin")

	e.Look("repo", "dvalin")

	st, _ := ps.GetState("dvalin")
	if st.Phase != worker.NotDone {
		t.Fatalf("state = %q, want %q — it is still reading", st.Phase, worker.NotDone)
	}
	if st.Task != "" {
		t.Errorf("nothing may be selected while it is still reading, got %q", st.Task)
	}
	if len(deps.Cleared) != 0 {
		t.Errorf("no clear may fire over a message being read, got %v", deps.Cleared)
	}
}

// TestFinishingWithTheMailReleasesTheAgent is the other half: back at an empty prompt with nothing
// unread is what "done" means, and the ordinary flow resumes from there.
func TestFinishingWithTheMailReleasesTheAgent(t *testing.T) {
	deps := &stubDeps{}
	e, ps := idleWorkerWithOpenTask(t, deps)
	deps.Turning("dvalin", true)
	addUnreadMail(t, ps, "dvalin")
	e.Look("repo", "dvalin")

	deps.Turning("dvalin", false) // the turn ended; it is back at its prompt
	e.Look("repo", "dvalin")

	if st, _ := ps.GetState("dvalin"); st.Task == "" {
		t.Errorf("once it has finished with its mail the hub selects work for it, got %+v", st)
	}
}
