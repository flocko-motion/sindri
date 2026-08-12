package registry

import (
	"io"
	"testing"
)

func names(cmds []Command) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Name
	}
	return out
}

func fixture() *Registry {
	noop := func(Caller, []string, io.Writer) (int, error) { return 0, nil }
	return New(
		Command{Name: "status", Help: "who am I", Run: noop},
		Command{Name: "submit", Help: "request merge", Roles: []string{"worker"}, Run: noop},
		Command{Name: "approve", Help: "approve a PR", Roles: []string{"reviewer"}, Run: noop},
		Command{Name: "reject", Help: "reject a PR", Roles: []string{"reviewer"}, Run: noop},
		Command{Name: "next", Help: "next task", Roles: []string{"worker"},
			Blocked: func(c Caller) string {
				if c.HasTask {
					return "you already hold work"
				}
				return ""
			}, Run: noop},
	)
}

// 2.7: worker surface excludes approve/reject (and merge, which is never
// registered); reviewer surface excludes submit.
func TestRoleFiltering(t *testing.T) {
	r := fixture()

	worker := names(r.Available(Caller{Role: "worker"}))
	for _, bad := range []string{"approve", "reject", "merge"} {
		if contains(worker, bad) {
			t.Fatalf("worker surface must not include %q: %v", bad, worker)
		}
	}
	if !contains(worker, "submit") {
		t.Fatalf("worker surface must include submit: %v", worker)
	}

	reviewer := names(r.Available(Caller{Role: "reviewer"}))
	for _, bad := range []string{"submit", "next", "merge"} {
		if contains(reviewer, bad) {
			t.Fatalf("reviewer surface must not include %q: %v", bad, reviewer)
		}
	}
	if !contains(reviewer, "approve") || !contains(reviewer, "reject") {
		t.Fatalf("reviewer surface must include approve+reject: %v", reviewer)
	}
}

// State machine: a worker holding a task hides "next".
func TestStateHidesNext(t *testing.T) {
	r := fixture()
	idle := names(r.Available(Caller{Role: "worker", HasTask: false}))
	busy := names(r.Available(Caller{Role: "worker", HasTask: true}))
	if !contains(idle, "next") {
		t.Fatalf("idle worker should see next: %v", idle)
	}
	if contains(busy, "next") {
		t.Fatalf("busy worker should NOT see next: %v", busy)
	}
}

// A verb of ANOTHER role is indistinguishable from unknown — role isolation, so a worker learns
// nothing of the reviewer's surface.
func TestResolveRespectsRoles(t *testing.T) {
	r := fixture()
	if _, reason, ok := r.Resolve("approve", Caller{Role: "worker"}); ok || reason != "" {
		t.Fatalf("reviewer-only 'approve' must read as unknown to a worker, got reason %q", reason)
	}
	if _, _, ok := r.Resolve("submit", Caller{Role: "worker"}); !ok {
		t.Fatalf("worker should resolve 'submit'")
	}
	if _, reason, ok := r.Resolve("nope", Caller{Role: "worker"}); ok || reason != "" {
		t.Fatalf("unknown command must not resolve, got reason %q", reason)
	}
}

// TestResolveExplainsAStateGate is the point of holding the reason rather than a boolean: a verb of
// the caller's OWN role, blocked by the state machine, must come back with why. Vanishing from the
// surface left an agent that had been told to run it with no way to learn what to run instead.
func TestResolveExplainsAStateGate(t *testing.T) {
	r := fixture()
	busy := Caller{Role: "worker", HasTask: true}
	if contains(names(r.Available(busy)), "next") {
		t.Fatal("a busy worker should not be OFFERED next")
	}
	cmd, reason, ok := r.Resolve("next", busy)
	if ok || cmd.Name != "" {
		t.Fatal("a busy worker must not be able to run next")
	}
	if reason == "" {
		t.Fatal("a state-blocked verb must explain itself rather than read as unknown")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
