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
	p := openTmp(t).For("repo")

	if _, ok, err := p.GetAgent("brokkr"); err != nil || ok {
		t.Fatalf("expected absent agent, got ok=%v err=%v", ok, err)
	}

	if err := p.PutAgent(Agent{Name: "brokkr", Role: "worker", Workspace: ".worktrees/brokkr"}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := p.PutAgent(Agent{Name: "dvalin", Role: "reviewer"}); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, ok, err := p.GetAgent("brokkr")
	if err != nil || !ok {
		t.Fatalf("get brokkr: ok=%v err=%v", ok, err)
	}
	if got.Role != "worker" || got.Workspace != ".worktrees/brokkr" || got.Project != "repo" {
		t.Fatalf("unexpected agent: %+v", got)
	}
	if got.CreatedAt == "" {
		t.Fatalf("created_at not stamped")
	}

	roster, err := p.Roster()
	if err != nil {
		t.Fatalf("roster: %v", err)
	}
	if len(roster) != 2 || roster[0].Name != "brokkr" || roster[1].Name != "dvalin" {
		t.Fatalf("unexpected roster order/size: %+v", roster)
	}
}

func TestPutPreservesCreatedAt(t *testing.T) {
	p := openTmp(t).For("repo")
	if err := p.PutAgent(Agent{Name: "brokkr", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	first, _, _ := p.GetAgent("brokkr")
	// Update role; created_at must survive.
	if err := p.PutAgent(Agent{Name: "brokkr", Role: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	second, _, _ := p.GetAgent("brokkr")
	if second.Role != "reviewer" {
		t.Fatalf("role not updated: %+v", second)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Fatalf("created_at changed on update: %q -> %q", first.CreatedAt, second.CreatedAt)
	}
}

func TestDeleteAgent(t *testing.T) {
	p := openTmp(t).For("repo")
	p.PutAgent(Agent{Name: "brokkr", Role: "worker"})
	if err := p.DeleteAgent("brokkr"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := p.GetAgent("brokkr"); ok {
		t.Fatalf("agent still present after delete")
	}
}

func TestEventsAppendAndOrder(t *testing.T) {
	p := openTmp(t).For("repo")
	for _, m := range []string{"first", "second", "third"} {
		if err := p.Log("brokkr", "inject", m); err != nil {
			t.Fatalf("log: %v", err)
		}
	}
	p.Log("dvalin", "inject", "other") // different agent, must not leak in

	all, err := p.Events("brokkr", 0)
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
	last2, err := p.Events("brokkr", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(last2) != 2 || last2[0].Payload != "second" || last2[1].Payload != "third" {
		t.Fatalf("limit/order wrong: %+v", last2)
	}
}

// Two projects may each hold an agent of the same name without collision, and one
// project's roster/events never leak into another's (task 7.1).
func TestProjectIsolation(t *testing.T) {
	s := openTmp(t)
	a, b := s.For("repoA"), s.For("repoB")

	if err := a.PutAgent(Agent{Name: "eitri", Role: "coauthor"}); err != nil {
		t.Fatal(err)
	}
	if err := b.PutAgent(Agent{Name: "eitri", Role: "worker"}); err != nil {
		t.Fatalf("same name in another project must be allowed: %v", err)
	}
	a.Log("eitri", "note", "in-A")
	b.Log("eitri", "note", "in-B")

	ea, _, _ := a.GetAgent("eitri")
	eb, _, _ := b.GetAgent("eitri")
	if ea.Role != "coauthor" || eb.Role != "worker" {
		t.Fatalf("cross-project bleed: A=%+v B=%+v", ea, eb)
	}
	if r, _ := a.Roster(); len(r) != 1 {
		t.Fatalf("repoA roster leaked: %+v", r)
	}
	if evs, _ := a.Events("eitri", 0); len(evs) != 1 || evs[0].Payload != "in-A" {
		t.Fatalf("repoA events wrong/leaked: %+v", evs)
	}

	all, _ := s.AllAgents()
	if len(all) != 2 {
		t.Fatalf("AllAgents across projects = %d, want 2", len(all))
	}
}

func TestProjectRegistry(t *testing.T) {
	s := openTmp(t)
	if err := s.RegisterProject("tagA", "/repos/a"); err != nil {
		t.Fatal(err)
	}
	s.RegisterProject("tagB", "/repos/b")
	if path, ok, err := s.ProjectPath("tagA"); err != nil || !ok || path != "/repos/a" {
		t.Fatalf("ProjectPath(tagA) = %q, %v, %v", path, ok, err)
	}
	if _, ok, err := s.ProjectPath("nope"); err != nil || ok {
		t.Fatalf("unknown tag resolved (ok=%v err=%v)", ok, err)
	}
	ps, _ := s.Projects()
	if len(ps) != 2 {
		t.Fatalf("Projects = %d, want 2", len(ps))
	}
	// RegisterProject stamps last_used (the switcher's recency signal).
	for _, p := range ps {
		if p.LastUsed == "" {
			t.Errorf("project %s has no last_used stamp", p.Tag)
		}
	}

	// UnregisterProject removes only that row — the other project stays.
	if err := s.UnregisterProject("tagA"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.ProjectPath("tagA"); ok {
		t.Error("forgotten project should be gone from the registry")
	}
	if _, ok, _ := s.ProjectPath("tagB"); !ok {
		t.Error("forget of tagA must not remove tagB")
	}
	if ps, _ := s.Projects(); len(ps) != 1 {
		t.Fatalf("after forget, Projects = %d, want 1", len(ps))
	}

	// Forget is transient: registering again re-adds it.
	if err := s.RegisterProject("tagA", "/repos/a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.ProjectPath("tagA"); !ok {
		t.Error("re-registered project should resolve again")
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
	s1.For("repo").PutAgent(Agent{Name: "brokkr", Role: "worker"})
	s1.For("repo").Log("brokkr", "launch", "started")
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, ok, _ := s2.For("repo").GetAgent("brokkr"); !ok {
		t.Fatalf("agent lost across reopen")
	}
	if evs, _ := s2.For("repo").Events("brokkr", 0); len(evs) != 1 {
		t.Fatalf("events lost across reopen: %d", len(evs))
	}
}

// An agent's memory limit round-trips through the store, and reopening the same DB
// (which re-runs the ALTER migration) is idempotent — not an error.
func TestAgentMemoryRoundTripAndMigrateIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hub.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ps := s.For("proj")
	if err := ps.PutAgent(Agent{Name: "brokkr", Role: "worker", Memory: "4g"}); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok, err := ps.GetAgent("brokkr")
	if err != nil || !ok || got.Memory != "4g" {
		t.Fatalf("GetAgent memory = %q (ok=%v err=%v), want 4g", got.Memory, ok, err)
	}
	s.Close()

	// Reopen: migrate() runs again and must not error on the already-present column.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen (migrate not idempotent?): %v", err)
	}
	defer s2.Close()
	got2, _, _ := s2.For("proj").GetAgent("brokkr")
	if got2.Memory != "4g" {
		t.Errorf("after reopen, memory = %q, want 4g", got2.Memory)
	}
}
