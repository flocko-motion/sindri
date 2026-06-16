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
			Hidden: func(c Caller) bool { return c.HasTask }, Run: noop},
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

// An out-of-surface verb is indistinguishable from unknown (invisible, not
// rejected).
func TestLookupRespectsSurface(t *testing.T) {
	r := fixture()
	if _, ok := r.Lookup("approve", Caller{Role: "worker"}); ok {
		t.Fatalf("worker must not resolve reviewer-only 'approve'")
	}
	if _, ok := r.Lookup("submit", Caller{Role: "worker"}); !ok {
		t.Fatalf("worker should resolve 'submit'")
	}
	if _, ok := r.Lookup("nope", Caller{Role: "worker"}); ok {
		t.Fatalf("unknown command must not resolve")
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
