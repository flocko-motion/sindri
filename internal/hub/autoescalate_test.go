package hub

import (
	"io"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestAnInternalErrorEscalatesTheAgentItself is what vestri's blind review cost: told "try again
// later", it retried, routed around the breakage and reviewed a PR without the task it implements.
// It knew the rule and followed the message in front of it, so the hub now escalates rather than ask.
func TestAnInternalErrorEscalatesTheAgentItself(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	// A missing worktree fails inside the hub: a returned error, which is the boundary this keys on.
	_, err := h.AgentExec(testProject, "dvalin", []string{"git", "status"}, io.Discard)
	if err == nil {
		t.Fatal("setup: expected an internal error from `git` against a missing worktree")
	}
	if !strings.Contains(err.Error(), "ESCALATED") {
		t.Errorf("the reply must say the agent is escalated, got: %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "try again") {
		t.Errorf("the reply must not invite a retry — that sentence caused the incident: %v", err)
	}
	st, serr := ps.GetState("dvalin")
	if serr != nil {
		t.Fatalf("read state: %v", serr)
	}
	if st.Escalation == "" {
		t.Fatal("an internal error must escalate the agent, so its landing verbs shut and the user sees it")
	}
	if !strings.Contains(st.Escalation, "git") {
		t.Errorf("the escalation must name the verb that failed, got %q", st.Escalation)
	}

	// A second failure must not overwrite the first: the question on record is the one that stopped it.
	first := st.Escalation
	_, _ = h.AgentExec(testProject, "dvalin", []string{"git", "diff"}, io.Discard)
	if again, _ := ps.GetState("dvalin"); again.Escalation != first {
		t.Errorf("a further failure rewrote the escalation: %q -> %q", first, again.Escalation)
	}
}

// TestARefusalDoesNotEscalate: a verb held back by the state machine is an ordinary answer, and
// escalating those would stop an agent every time it asked for something it cannot have yet.
func TestARefusalDoesNotEscalate(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	// A verb this role does not have: refused on `out`, with a nil error.
	if _, err := h.AgentExec(testProject, "dvalin", []string{"approve", "pr-1"}, io.Discard); err != nil {
		t.Fatalf("a refusal must not return an error: %v", err)
	}
	if st, _ := ps.GetState("dvalin"); st.Escalation != "" {
		t.Errorf("a refusal escalated the agent: %q", st.Escalation)
	}
}
