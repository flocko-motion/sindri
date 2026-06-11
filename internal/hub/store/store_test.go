package store

import (
	"path/filepath"
	"testing"
)

func openTmp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutGetRoster(t *testing.T) {
	s := openTmp(t)

	if _, ok, err := s.GetAgent("brokkr"); err != nil || ok {
		t.Fatalf("expected absent agent, got ok=%v err=%v", ok, err)
	}

	if err := s.PutAgent(Agent{Name: "brokkr", Role: "worker", Workspace: ".worktrees/brokkr"}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := s.PutAgent(Agent{Name: "dvalin", Role: "reviewer"}); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, ok, err := s.GetAgent("brokkr")
	if err != nil || !ok {
		t.Fatalf("get brokkr: ok=%v err=%v", ok, err)
	}
	if got.Role != "worker" || got.Workspace != ".worktrees/brokkr" {
		t.Fatalf("unexpected agent: %+v", got)
	}
	if got.CreatedAt == "" {
		t.Fatalf("created_at not stamped")
	}

	roster, err := s.Roster()
	if err != nil {
		t.Fatalf("roster: %v", err)
	}
	if len(roster) != 2 || roster[0].Name != "brokkr" || roster[1].Name != "dvalin" {
		t.Fatalf("unexpected roster order/size: %+v", roster)
	}
}

func TestPutPreservesCreatedAt(t *testing.T) {
	s := openTmp(t)
	if err := s.PutAgent(Agent{Name: "brokkr", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	first, _, _ := s.GetAgent("brokkr")
	// Update role; created_at must survive.
	if err := s.PutAgent(Agent{Name: "brokkr", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	second, _, _ := s.GetAgent("brokkr")
	if second.Role != "reviewer" {
		t.Fatalf("role not updated: %+v", second)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Fatalf("created_at changed on update: %q -> %q", first.CreatedAt, second.CreatedAt)
	}
}

func TestDeleteAgent(t *testing.T) {
	s := openTmp(t)
	s.PutAgent(Agent{Name: "brokkr", Role: "worker"})
	if err := s.DeleteAgent("brokkr"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.GetAgent("brokkr"); ok {
		t.Fatalf("agent still present after delete")
	}
}

func TestEventsAppendAndOrder(t *testing.T) {
	s := openTmp(t)
	for _, m := range []string{"first", "second", "third"} {
		if err := s.Log("brokkr", "inject", m); err != nil {
			t.Fatalf("log: %v", err)
		}
	}
	s.Log("dvalin", "inject", "other") // different agent, must not leak in

	all, err := s.Events("brokkr", 0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 events, got %d", len(all))
	}
	if all[0].Payload != "first" || all[2].Payload != "third" {
		t.Fatalf("events not oldest-first: %+v", all)
	}

	// limit returns the most-recent N, still oldest-first.
	last2, err := s.Events("brokkr", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(last2) != 2 || last2[0].Payload != "second" || last2[1].Payload != "third" {
		t.Fatalf("limit/order wrong: %+v", last2)
	}
}

// Reopening the same file must recover all committed state (crash-restart, D11).
func TestDurableAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hub.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s1.PutAgent(Agent{Name: "brokkr", Role: "worker"})
	s1.Log("brokkr", "launch", "started")
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, ok, _ := s2.GetAgent("brokkr"); !ok {
		t.Fatalf("agent lost across reopen")
	}
	if evs, _ := s2.Events("brokkr", 0); len(evs) != 1 {
		t.Fatalf("events lost across reopen: %d", len(evs))
	}
}
