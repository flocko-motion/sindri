package store

import "testing"

// TestChatMembership exercises the round-trip: add is idempotent, membership is
// reported, remove tells you whether it removed anything, members carry role from
// the agents table, and deleting an agent drops its membership.
func TestChatMembership(t *testing.T) {
	s := openTmp(t)
	p := s.For("repo")
	if err := p.PutAgent(Agent{Name: "nori", Role: "worker"}); err != nil {
		t.Fatalf("put: %v", err)
	}

	if m, err := s.ChatIsMember("repo", "nori"); err != nil || m {
		t.Fatalf("expected non-member, got m=%v err=%v", m, err)
	}
	if err := s.ChatAdd("repo", "nori"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := s.ChatAdd("repo", "nori"); err != nil { // idempotent
		t.Fatalf("re-add should be a no-op, got: %v", err)
	}
	if m, err := s.ChatIsMember("repo", "nori"); err != nil || !m {
		t.Fatalf("expected member, got m=%v err=%v", m, err)
	}

	members, err := s.ChatMembers()
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	if len(members) != 1 || members[0].Name != "nori" || members[0].Role != "worker" {
		t.Fatalf("members = %+v, want one nori (worker)", members)
	}

	// Removing a member reports true; removing again reports false.
	if was, err := s.ChatRemove("repo", "nori"); err != nil || !was {
		t.Fatalf("remove: was=%v err=%v, want was=true", was, err)
	}
	if was, err := s.ChatRemove("repo", "nori"); err != nil || was {
		t.Fatalf("second remove: was=%v err=%v, want was=false", was, err)
	}

	// DeleteAgent must also drop membership (no phantom members).
	if err := s.ChatAdd("repo", "nori"); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if err := p.DeleteAgent("nori"); err != nil {
		t.Fatalf("delete agent: %v", err)
	}
	if m, err := s.ChatIsMember("repo", "nori"); err != nil || m {
		t.Fatalf("membership should be gone after DeleteAgent, got m=%v err=%v", m, err)
	}
}

// TestChatTranscript checks append ordering, monotonic ids, and the tail limit.
func TestChatTranscript(t *testing.T) {
	s := openTmp(t)
	for _, body := range []string{"one", "two", "three"} {
		if _, err := s.ChatAppend("user", body); err != nil {
			t.Fatalf("append %q: %v", body, err)
		}
	}
	all, err := s.ChatTranscript(0)
	if err != nil {
		t.Fatalf("transcript: %v", err)
	}
	if len(all) != 3 || all[0].Body != "one" || all[2].Body != "three" {
		t.Fatalf("transcript = %+v, want one/two/three in order", all)
	}
	if all[0].ID >= all[1].ID || all[1].ID >= all[2].ID {
		t.Fatalf("ids must be monotonic, got %d/%d/%d", all[0].ID, all[1].ID, all[2].ID)
	}
	// A limit returns the newest N, still oldest-first.
	tail, err := s.ChatTranscript(2)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(tail) != 2 || tail[0].Body != "two" || tail[1].Body != "three" {
		t.Fatalf("tail = %+v, want two/three", tail)
	}
}
