package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

func TestAgentTokenDeterministicAndDistinct(t *testing.T) {
	h, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	tok, err := h.AgentToken("eitri")
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	again, _ := h.AgentToken("eitri")
	if again != tok {
		t.Errorf("token not deterministic: %q vs %q", tok, again)
	}
	other, _ := h.AgentToken("dvalin")
	if other == tok {
		t.Error("different agents share a token")
	}
}

func TestAgentForToken(t *testing.T) {
	h, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if err := h.store.PutAgent(store.Agent{Name: "eitri", Role: "coauthor"}); err != nil {
		t.Fatal(err)
	}
	tok, _ := h.AgentToken("eitri")

	if name, ok := h.agentForToken(tok); !ok || name != "eitri" {
		t.Errorf("agentForToken(valid) = (%q, %v), want (eitri, true)", name, ok)
	}
	if _, ok := h.agentForToken("deadbeef"); ok {
		t.Error("a bogus token resolved to an agent")
	}
	if _, ok := h.agentForToken(""); ok {
		t.Error("an empty token resolved to an agent")
	}
	// A well-formed token for an agent NOT on the roster must not resolve.
	dvalin, _ := h.AgentToken("dvalin")
	if _, ok := h.agentForToken(dvalin); ok {
		t.Error("token for an unrostered agent resolved")
	}
}

func TestAgentTokenStableAcrossRestart(t *testing.T) {
	root := t.TempDir()
	h1, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	tok1, _ := h1.AgentToken("eitri")
	h1.Close()

	// A fresh hub on the same repo (a restart) must derive the identical token, or
	// it would fail to authenticate an already-running pod (D11).
	h2, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer h2.Close()
	tok2, _ := h2.AgentToken("eitri")
	if tok1 != tok2 {
		t.Errorf("token changed across restart: %q vs %q", tok1, tok2)
	}
}
