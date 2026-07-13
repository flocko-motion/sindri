package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

func TestAgentTokenDeterministicAndDistinct(t *testing.T) {
	h := newHub(t)

	tok, err := h.agents.Token("proj", "eitri")
	if err != nil || tok == "" {
		t.Fatalf("AgentToken: %q err=%v", tok, err)
	}
	again, _ := h.agents.Token("proj", "eitri")
	if again != tok {
		t.Errorf("token not deterministic: %q vs %q", tok, again)
	}
	if other, _ := h.agents.Token("proj", "dvalin"); other == tok {
		t.Error("different agents share a token")
	}
	// Same name in another project must get a distinct token.
	if crossProject, _ := h.agents.Token("proj2", "eitri"); crossProject == tok {
		t.Error("same name across projects shares a token")
	}
}

func TestAgentForToken(t *testing.T) {
	h := newHub(t)
	if err := h.store.For("proj").PutAgent(store.Agent{Name: "eitri", Role: "coauthor"}); err != nil {
		t.Fatal(err)
	}
	tok, _ := h.agents.Token("proj", "eitri")

	if p, n, ok, err := h.agents.ForToken(tok); err != nil || !ok || p != "proj" || n != "eitri" {
		t.Errorf("agentForToken(valid) = (%q, %q, %v, %v), want (proj, eitri, true, nil)", p, n, ok, err)
	}
	if _, _, ok, err := h.agents.ForToken("deadbeef"); err != nil || ok {
		t.Errorf("a bogus token resolved to an agent (ok=%v err=%v)", ok, err)
	}
	if _, _, ok, err := h.agents.ForToken(""); err != nil || ok {
		t.Errorf("an empty token resolved to an agent (ok=%v err=%v)", ok, err)
	}
	// A well-formed token for an agent NOT on the roster must not resolve.
	dvalin, _ := h.agents.Token("proj", "dvalin")
	if _, _, ok, err := h.agents.ForToken(dvalin); err != nil || ok {
		t.Errorf("token for an unrostered agent resolved (ok=%v err=%v)", ok, err)
	}
}

func TestAgentTokenStableAcrossRestart(t *testing.T) {
	t.Setenv("SINDRI_HOME", t.TempDir())
	h1, err := New()
	if err != nil {
		t.Fatal(err)
	}
	tok1, _ := h1.agents.Token("proj", "eitri")
	h1.Close()

	// A fresh hub (a restart) must derive the identical token, or it would fail to
	// authenticate an already-running pod (D11).
	h2, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer h2.Close()
	tok2, _ := h2.agents.Token("proj", "eitri")
	if tok1 != tok2 {
		t.Errorf("token changed across restart: %q vs %q", tok1, tok2)
	}
}
