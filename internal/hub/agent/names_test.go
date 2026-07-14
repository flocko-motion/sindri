package agent

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// newService opens the agent-actor service over a throwaway store.
func newService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	// nil deps/channel: these tests exercise only store-backed helpers (naming, tokens),
	// which never reach the deps or the agent channel.
	return New(st, nil, nil), st
}

func inPool(name string) bool {
	for _, d := range dwarfNames {
		if d == name {
			return true
		}
	}
	return false
}

func TestAutoNameFromPoolAndUnique(t *testing.T) {
	s, st := newService(t)

	// A fresh store hands out a dwarf from the pool — never a binary name.
	n, err := s.AutoName()
	if err != nil {
		t.Fatal(err)
	}
	if !inPool(n) {
		t.Fatalf("auto-name %q is not from the dwarf pool", n)
	}
	if n == "sindri" || n == "brokkr" {
		t.Fatalf("must never hand out a binary name, got %q", n)
	}

	// Once that name is taken (in ANY project), AutoName must not reuse it.
	if err := st.For("proj").PutAgent(store.Agent{Name: n, Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	if n2, err := s.AutoName(); err != nil {
		t.Fatal(err)
	} else if n2 == n {
		t.Fatalf("AutoName returned the taken name %q", n)
	}
}
