package fleet

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/roles/worker"
	"github.com/flo-at/sindri/internal/hub/store"
)

// deliveredContaining reports whether any message the stub was handed carries needle.
func deliveredContaining(d *stubDeps, needle string) bool {
	for _, text := range d.injectedText {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// addUnreadMail seeds one unread message for an agent and returns its id.
func addUnreadMail(t *testing.T, ps *store.ProjectStore, agent string) int64 {
	t.Helper()
	m, err := ps.AddMail(agent, "hub", "[hub] something happened", false, 0)
	if err != nil {
		t.Fatalf("AddMail: %v", err)
	}
	return m.ID
}

// TestUnreadMailStopsAnAgentBeingPrepared is the invariant that replaced eight tests of a deferral
// mechanism. Those deferred mail PAST a clear so it would not be marked read into a session about
// to be discarded — machinery for a case that should never arise. Unread mail means the agent is
// NOT DONE, and only a done agent is prepared for new work, so the state a clear fires from is
// simply unreachable while the mailbox has something in it.
func TestUnreadMailStopsAnAgentBeingPrepared(t *testing.T) {
	deps := &stubDeps{alive: true, ctxTokens: 900_000, ctxWindow: 1_000_000, ctxOK: true}
	e, ps := idleWorkerWithOpenTask(t, deps)
	addUnreadMail(t, ps, "dvalin")

	e.Look("repo", "dvalin")

	st, _ := ps.GetState("dvalin")
	if st.Phase != worker.Mail {
		t.Fatalf("state = %q, want %q — unread mail means it is not done", st.Phase, worker.Mail)
	}
	if st.Task != "" {
		t.Errorf("nothing may be claimed for an agent that has not read its mail, got %q", st.Task)
	}
	if len(deps.cleared) != 0 {
		t.Errorf("no clear may fire over unread mail, got %v", deps.cleared)
	}
}

// TestReadingTheMailReleasesTheAgent is the other half: the mailbox emptying is the exit, and the
// ordinary flow resumes at once rather than at some later beat.
func TestReadingTheMailReleasesTheAgent(t *testing.T) {
	deps := &stubDeps{alive: true}
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

// TestTheAskServesTheMailItIsHeldFor: the agent is told it has mail AND handed the mail in the same
// answer — a state that says "you have mail" without carrying it would be a round trip for nothing.
func TestTheAskServesTheMailItIsHeldFor(t *testing.T) {
	deps := &stubDeps{alive: true}
	e, ps := idleWorkerWithOpenTask(t, deps)
	addUnreadMail(t, ps, "dvalin")

	dir, err := e.AgentDirective(t.Context(), "repo", "dvalin")
	if err != nil {
		t.Fatalf("AgentDirective: %v", err)
	}
	if !strings.Contains(dir, "something happened") {
		t.Errorf("the answer must carry the message itself: %q", dir)
	}
	if n, _ := ps.UnreadMailCount("dvalin"); n != 0 {
		t.Errorf("serving the mail marks it read, got %d unread", n)
	}
}
