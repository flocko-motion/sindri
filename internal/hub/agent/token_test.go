package agent

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

func TestTokenDeterministicAndDistinct(t *testing.T) {
	s, _ := newService(t)
	tok, err := s.Token("proj", "eitri")
	if err != nil || tok == "" {
		t.Fatalf("Token: %q err=%v", tok, err)
	}
	if again, _ := s.Token("proj", "eitri"); again != tok {
		t.Errorf("token not deterministic: %q vs %q", tok, again)
	}
	if other, _ := s.Token("proj", "dvalin"); other == tok {
		t.Error("different agents share a token")
	}
	// Same name in another project must get a distinct token.
	if cross, _ := s.Token("proj2", "eitri"); cross == tok {
		t.Error("same name across projects shares a token")
	}
}

func TestForToken(t *testing.T) {
	s, st := newService(t)
	if err := st.For("proj").PutAgent(store.Agent{Name: "eitri", Role: "coauthor"}); err != nil {
		t.Fatal(err)
	}
	tok, _ := s.Token("proj", "eitri")

	if p, n, ok, err := s.ForToken(tok); err != nil || !ok || p != "proj" || n != "eitri" {
		t.Errorf("ForToken(valid) = (%q, %q, %v, %v), want (proj, eitri, true, nil)", p, n, ok, err)
	}
	if _, _, ok, err := s.ForToken("deadbeef"); err != nil || ok {
		t.Errorf("a bogus token resolved to an agent (ok=%v err=%v)", ok, err)
	}
	if _, _, ok, err := s.ForToken(""); err != nil || ok {
		t.Errorf("an empty token resolved to an agent (ok=%v err=%v)", ok, err)
	}
	// A well-formed token for an agent NOT on the roster must not resolve.
	dvalin, _ := s.Token("proj", "dvalin")
	if _, _, ok, err := s.ForToken(dvalin); err != nil || ok {
		t.Errorf("token for an unrostered agent resolved (ok=%v err=%v)", ok, err)
	}
}

func TestTokenStableAcrossRestart(t *testing.T) {
	// The secret is persisted in the store, so a fresh service over a REOPENED store
	// (a hub restart) derives the identical token — or it would fail to authenticate an
	// already-running pod (D11).
	path := filepath.Join(t.TempDir(), "hub.db")
	st1, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	tok1, _ := New(st1, nil).Token("proj", "eitri")
	st1.Close()

	st2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	tok2, _ := New(st2, nil).Token("proj", "eitri")
	if tok1 != tok2 {
		t.Errorf("token changed across restart: %q vs %q", tok1, tok2)
	}
}
