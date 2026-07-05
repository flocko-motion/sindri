package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

func TestAgentTokenDeterministicAndDistinct(t *testing.T) {
	h := newHub(t)

	tok, err := h.AgentToken("proj", "eitri")
	if err != nil || tok == "" {
		t.Fatalf("AgentToken: %q err=%v", tok, err)
	}
	again, _ := h.AgentToken("proj", "eitri")
	if again != tok {
		t.Errorf("token not deterministic: %q vs %q", tok, again)
	}
	if other, _ := h.AgentToken("proj", "dvalin"); other == tok {
		t.Error("different agents share a token")
	}
	// Same name in another project must get a distinct token.
	if crossProject, _ := h.AgentToken("proj2", "eitri"); crossProject == tok {
		t.Error("same name across projects shares a token")
	}
}

func TestAgentForToken(t *testing.T) {
	h := newHub(t)
	if err := h.store.For("proj").PutAgent(store.Agent{Name: "eitri", Role: "coauthor"}); err != nil {
		t.Fatal(err)
	}
	tok, _ := h.AgentToken("proj", "eitri")

	if p, n, ok := h.agentForToken(tok); !ok || p != "proj" || n != "eitri" {
		t.Errorf("agentForToken(valid) = (%q, %q, %v), want (proj, eitri, true)", p, n, ok)
	}
	if _, _, ok := h.agentForToken("deadbeef"); ok {
		t.Error("a bogus token resolved to an agent")
	}
	if _, _, ok := h.agentForToken(""); ok {
		t.Error("an empty token resolved to an agent")
	}
	// A well-formed token for an agent NOT on the roster must not resolve.
	dvalin, _ := h.AgentToken("proj", "dvalin")
	if _, _, ok := h.agentForToken(dvalin); ok {
		t.Error("token for an unrostered agent resolved")
	}
}

func TestAgentTokenStableAcrossRestart(t *testing.T) {
	t.Setenv("SINDRI_HOME", t.TempDir())
	h1, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tok1, _ := h1.AgentToken("proj", "eitri")
	h1.Close()

	// A fresh hub (a restart) must derive the identical token, or it would fail to
	// authenticate an already-running pod (D11).
	h2, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer h2.Close()
	tok2, _ := h2.AgentToken("proj", "eitri")
	if tok1 != tok2 {
		t.Errorf("token changed across restart: %q vs %q", tok1, tok2)
	}
}
