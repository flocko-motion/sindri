package hub

import (
	"errors"
	"io"
	"os"
	"path/filepath"
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
	_, err := h.AgentExec(t.Context(), testProject, "dvalin", []string{"git", "status"}, io.Discard)
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
	_, _ = h.AgentExec(t.Context(), testProject, "dvalin", []string{"git", "diff"}, io.Discard)
	if again, _ := ps.GetState("dvalin"); again.Escalation != first {
		t.Errorf("a further failure rewrote the escalation: %q -> %q", first, again.Escalation)
	}
}

// TestUnknownAgentIsARefusalNotAnInternalFailure is the round-3 rejection's finding on caller's own
// not-found branch: it is an identity answer (a stale name, a deleted agent, a pod outliving its
// registration), not a breakage, so escalating it would write a state row for an agent with no
// roster entry at all — invisible on the board, with nobody actually asked anything.
func TestUnknownAgentIsARefusalNotAnInternalFailure(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	_, err := h.AgentExec(t.Context(), testProject, "ghost", []string{"status"}, io.Discard)
	if err == nil {
		t.Fatal("an unknown agent must still be refused")
	}
	if strings.Contains(err.Error(), "ESCALATED") {
		t.Errorf("an unknown agent must not be escalated: %v", err)
	}
	// The exact string, not just Contains: round-4's rejection caught a stutter
	// ("unknown agent \"ghost\": unknown agent") that a looser assertion passed over.
	if err.Error() != `unknown agent "ghost"` {
		t.Errorf(`the refusal should read exactly "unknown agent %q", got: %v`, "ghost", err)
	}
	if st, _ := ps.GetState("ghost"); st.Escalation != "" {
		t.Errorf("a phantom state row must not appear for an unknown agent: %q", st.Escalation)
	}
}

// TestIdentityFailureAlsoGoesThroughInternalFailure is round-4's finding 4: caller's OTHER error path
// — a real store failure resolving identity, distinct from the not-found case above — must still
// reach internalFailure rather than be swallowed as an identity refusal. Forced by closing the store
// after registering the agent, so GetAgent itself fails; there is no cleaner way to fail just this
// one read against the real store.
func TestIdentityFailureAlsoGoesThroughInternalFailure(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := h.store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	_, err := h.AgentExec(t.Context(), testProject, "dvalin", []string{"status"}, io.Discard)
	if err == nil {
		t.Fatal("a store failure resolving identity must still return an error")
	}
	if strings.Contains(err.Error(), "unknown agent") {
		t.Errorf("a real store failure must not be reported as an identity refusal: %v", err)
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
	if _, err := h.AgentExec(t.Context(), testProject, "dvalin", []string{"approve", "pr-1"}, io.Discard); err != nil {
		t.Fatalf("a refusal must not return an error: %v", err)
	}
	if st, _ := ps.GetState("dvalin"); st.Escalation != "" {
		t.Errorf("a refusal escalated the agent: %q", st.Escalation)
	}
}

// TestInternalFailureDoesNotOverwriteALiveEscalation is the review's stale-snapshot regression from
// the `escalate` side: a caller snapshot taken before Run cannot be trusted once Run may have raised
// the very escalation being checked, so the decision has to key on a fresh read. Called directly
// (rather than reproducing the exact SetEscalation-succeeds-then-Log-fails trigger, which the real
// store gives no clean way to force) because that fresh read is the whole of what internalFailure
// has to get right — it takes no caller snapshot at all, so this is what "live, not stale" reduces to.
func TestInternalFailureDoesNotOverwriteALiveEscalation(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if _, err := h.Escalate(testProject, "dvalin", "my own question"); err != nil {
		t.Fatalf("escalate: %v", err)
	}
	_, err := h.internalFailure(testProject, "dvalin", "`sindri escalate`", 1, errors.New("boom"))
	if strings.Contains(err.Error(), "the hub reported it for you") {
		t.Errorf("Escalate was skipped here (already live) — this fault was never reported, and the "+
			"reply must not claim it was: %v", err)
	}
	if !strings.Contains(err.Error(), "sindri log") {
		t.Errorf("the reply should point at `sindri log`, still open, to put this fault on record: %v", err)
	}
	if st, _ := ps.GetState("dvalin"); st.Escalation != "my own question" {
		t.Errorf("a live escalation must not be overwritten, got %q", st.Escalation)
	}
}

// TestInternalFailureEscalatesFreshAfterAGenuineResume is the mirror, from the `resume` side: once
// live state is actually clear, a subsequent internal failure must escalate for real (with a new
// reason), not report ESCALATED while leaving the agent's landing verbs open.
func TestInternalFailureEscalatesFreshAfterAGenuineResume(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if _, err := h.Escalate(testProject, "dvalin", "old question"); err != nil {
		t.Fatalf("escalate: %v", err)
	}
	if err := h.Resume(testProject, "dvalin", "answered"); err != nil {
		t.Fatalf("resume: %v", err)
	}
	_, err := h.internalFailure(testProject, "dvalin", "`sindri git`", 1, errors.New("boom"))
	if !strings.Contains(err.Error(), "ESCALATED") {
		t.Errorf("a fresh internal failure should still escalate and say so: %v", err)
	}
	st, _ := ps.GetState("dvalin")
	if st.Escalation == "" {
		t.Fatal("a genuinely un-escalated agent must be escalated by a fresh internal failure")
	}
	if st.Escalation == "old question" {
		t.Errorf("the new escalation must not be the stale pre-resume reason, got %q", st.Escalation)
	}
}

// TestConfigErrorEscalatesToo: a broken project config is the one failure the code already knows
// only a human can end, and it must not be the one internal failure that still asks the agent to
// relay it — it goes through the very same door as every other returned error, with its own reason.
func TestConfigErrorEscalatesToo(t *testing.T) {
	h := newHub(t)
	root := t.TempDir()
	ps := h.repo(root)
	if err := ps.PutAgent(store.Agent{Name: "dvalin", Role: "worker", Workspace: ".worktrees/dvalin"}); err != nil {
		t.Fatalf("put agent: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".sindri"), 0o755); err != nil {
		t.Fatal(err)
	}
	// An unknown top-level key fails KnownFields(true) decoding — a malformed config, not an absent one.
	if err := os.WriteFile(filepath.Join(root, ".sindri", "config.yaml"), []byte("bogus_key: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := repoTag(root)
	_, err := h.AgentExec(t.Context(), project, "dvalin", []string{"git", "change"}, io.Discard)
	if err == nil {
		t.Fatal("expected the config error to surface")
	}
	if !strings.Contains(err.Error(), "ESCALATED") {
		t.Errorf("a broken project config should escalate like any other internal failure: %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "carry on with whatever") {
		t.Errorf("must not invite routing around it, the framing this task removed: %v", err)
	}
	st, _ := ps.GetState("dvalin")
	if st.Escalation == "" {
		t.Fatal("a broken project config must escalate the agent, not just report the error")
	}
	if !strings.Contains(st.Escalation, "keep failing until a human fixes it") {
		t.Errorf("the config-specific reason should say so, not the generic one: %q", st.Escalation)
	}
}
