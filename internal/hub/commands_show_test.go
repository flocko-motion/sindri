package hub

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestShowNeverMasksAnUnrecognisedIDAsAnInternalError is the crash this subtask exists to fix, run
// through the real AgentExec path — execAs fatals on any non-nil error, which is exactly how the
// bug surfaced: any id show could not resolve reached AgentExec's "an internal error running..."
// wrapper (-> commands.go), rather than a plain, agent-actionable refusal.
func TestShowNeverMasksAnUnrecognisedIDAsAnInternalError(t *testing.T) {
	h := newHub(t)
	if err := h.store.For(testProject).PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ml-465", "pr-nope", "wibble"} {
		out, code := execAs(t, h, "dvalin", "show", id)
		if code == 0 {
			t.Errorf("show %s: an unresolvable id must not report success", id)
		}
		if strings.Contains(out, "internal error") {
			t.Errorf("show %s: leaked the internal-error wrapper instead of a plain refusal: %s", id, out)
		}
	}
}

// TestShowMailReadsWhatWasSentToSomeoneElse: the read-by-id half of this subtask, exercised through
// AgentExec rather than the Engine directly, so it also proves AgentExec's error handling stays out
// of the way of a successful read.
func TestShowMailReadsWhatWasSentToSomeoneElse(t *testing.T) {
	h := newHub(t)
	for _, a := range []store.Agent{
		{Name: "dvalin", Role: "worker", Workspace: "ws"},
		{Name: "galar", Role: "planner", Workspace: "ws"},
	} {
		if err := h.store.For(testProject).PutAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	m, err := h.store.For(testProject).AddMail("galar", "hub", "a note meant for galar", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, code := execAs(t, h, "dvalin", "show", api.MailID(m.ID))
	if code != 0 {
		t.Fatalf("show %s: %d: %s", api.MailID(m.ID), code, out)
	}
	if !strings.Contains(out, "a note meant for galar") {
		t.Errorf("should print the body: %s", out)
	}
	stored, ok, err := h.store.MailByID(m.ID)
	if err != nil || !ok {
		t.Fatalf("MailByID(%d): ok=%v err=%v", m.ID, ok, err)
	}
	if stored.Read() {
		t.Error("show ml-<id> must not mark another agent's mail read")
	}
}
