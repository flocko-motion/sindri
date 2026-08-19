package hub

import (
	"fmt"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestPRListShowsOnlyAWorkersOwnPRs: a worker's question is "what did I submit, and what happened
// to it" — another worker's PR is neither actionable nor informative, so it must not appear.
func TestPRListShowsOnlyAWorkersOwnPRs(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-mine", Task: "td-1", Agent: "eitri", Branch: "td-1", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-theirs", Task: "td-2", Agent: "gimli", Branch: "td-2", Status: "open"}); err != nil {
		t.Fatal(err)
	}

	out, code := execAs(t, h, "eitri", "prs")
	if code != 0 {
		t.Fatalf("prs: code=%d out=%s", code, out)
	}
	if !strings.Contains(out, "pr-mine") {
		t.Errorf("a worker should see its own PR: %s", out)
	}
	if strings.Contains(out, "pr-theirs") {
		t.Errorf("a worker must not see another agent's PR: %s", out)
	}
	// "in total" on a scoped listing would read as the project's whole shape rather than just this
	// worker's — the footer must say whose count this is.
	if !strings.Contains(out, "of your active PR") || strings.Contains(out, "in total") {
		t.Errorf("a worker's footer should say the count is of its own PRs, not \"in total\": %s", out)
	}
	if !strings.Contains(out, "closed of yours") {
		t.Errorf("a worker's footer should close on \"of yours\", not \"in total\": %s", out)
	}
}

// TestPRListShowsEveryonesToOtherRoles: a reviewer authors nothing, so scoping by author would show
// it nothing; planners and coauthors read across the fleet the same way.
func TestPRListShowsEveryonesToOtherRoles(t *testing.T) {
	for _, role := range []string{"reviewer", "planner", "coauthor"} {
		t.Run(role, func(t *testing.T) {
			h := newHub(t)
			ps := h.store.For(testProject)
			if err := ps.PutAgent(store.Agent{Name: "rune", Role: role, Workspace: "ws"}); err != nil {
				t.Fatal(err)
			}
			if err := ps.PutPR(store.PR{ID: "pr-a", Task: "td-1", Agent: "eitri", Branch: "td-1", Status: "open"}); err != nil {
				t.Fatal(err)
			}
			if err := ps.PutPR(store.PR{ID: "pr-b", Task: "td-2", Agent: "gimli", Branch: "td-2", Status: "open"}); err != nil {
				t.Fatal(err)
			}

			out, code := execAs(t, h, "rune", "prs")
			if code != 0 {
				t.Fatalf("prs: code=%d out=%s", code, out)
			}
			if !strings.Contains(out, "pr-a") || !strings.Contains(out, "pr-b") {
				t.Errorf("a %s should see every PR in the project: %s", role, out)
			}
		})
	}
}

// TestPRListDefaultsToTenAndWidensWithLimit: a limit over an undefined order drops arbitrary rows,
// so the point here is the COUNT shown and the footer naming what the default left out.
func TestPRListDefaultsToTenAndWidensWithLimit(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "rune", Role: "reviewer", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		id := "pr-" + string(rune('a'+i))
		if err := ps.PutPR(store.PR{ID: id, Task: id, Agent: "eitri", Branch: id, Status: "open"}); err != nil {
			t.Fatal(err)
		}
	}

	out, code := execAs(t, h, "rune", "prs")
	if code != 0 {
		t.Fatalf("prs: code=%d out=%s", code, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 11 { // 10 PR rows + the summary footer
		t.Fatalf("got %d line(s), want 10 PR rows plus a footer: %q", len(lines), out)
	}
	footer := lines[len(lines)-1]
	if !strings.Contains(footer, "showing 10 of 12 active PRs") || !strings.Contains(footer, "--limit 12") {
		t.Errorf("footer should name the cap and how to widen it: %q", footer)
	}
	if !strings.Contains(footer, "12 open, 0 closed in total") {
		t.Errorf("footer should state the full open/closed shape: %q", footer)
	}

	out, code = execAs(t, h, "rune", "prs", "--limit", "20")
	if code != 0 {
		t.Fatalf("prs --limit 20: code=%d out=%s", code, out)
	}
	lines = strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 13 { // all 12 PR rows + the footer
		t.Fatalf("got %d line(s), want all 12 PR rows plus a footer: %q", len(lines), out)
	}
	if !strings.Contains(lines[len(lines)-1], "showing 12 active PRs") {
		t.Errorf("a wide-enough limit should say so without naming a cap: %q", lines[len(lines)-1])
	}
}

// TestPRListCapsNewestFirst pins the order the cap depends on: shown is a prefix of "recent", not an
// arbitrary limit-N of whatever store.PRs happens to hand back. Insertion order here (pr-a first)
// and id order agree with each other but DISAGREE with created_at order, so the two sets of survivors
// they would each produce are disjoint — the assertions below can only pass under created_at DESC.
func TestPRListCapsNewestFirst(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "rune", Role: "reviewer", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	// Inserted pr-a first, pr-l last, but stamped pr-a oldest through pr-l newest — the opposite of
	// insertion order. created_at DESC keeps pr-l..pr-c and drops pr-a/pr-b; insertion or id order
	// would instead keep pr-a..pr-j and drop pr-k/pr-l.
	for i := 0; i < 12; i++ {
		id := "pr-" + string(rune('a'+i))
		created := fmt.Sprintf("2026-01-01T00:00:%02dZ", i)
		if err := ps.PutPR(store.PR{ID: id, Task: id, Agent: "eitri", Branch: id, Status: "open", CreatedAt: created}); err != nil {
			t.Fatal(err)
		}
	}

	out, code := execAs(t, h, "rune", "prs")
	if code != 0 {
		t.Fatalf("prs: code=%d out=%s", code, out)
	}
	for _, newest := range []string{"pr-l", "pr-k", "pr-j"} { // most recently created (highest timestamp)
		if !strings.Contains(out, newest) {
			t.Errorf("the default cap should keep the newest PRs, missing %s: %s", newest, out)
		}
	}
	for _, oldest := range []string{"pr-a", "pr-b"} { // least recently created, dropped by the cap
		if strings.Contains(out, oldest) {
			t.Errorf("the default cap should drop the oldest PRs, found %s: %s", oldest, out)
		}
	}
}

// TestPRListRefusesAnUnknownArgument: swallowing an unrecognised flag is how --limit's own typo
// would go unnoticed — the same bug this fix exists to remove, one layer up.
func TestPRListRefusesAnUnknownArgument(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}

	out, code := execAs(t, h, "eitri", "prs", "--bogus")
	if code != 2 {
		t.Fatalf("code = %d, want 2 for an unrecognised argument: %s", code, out)
	}
	if !strings.Contains(out, `unknown argument "--bogus"`) || !strings.Contains(out, "usage: prs") {
		t.Errorf("should refuse by name and show usage: %q", out)
	}

	out, code = execAs(t, h, "eitri", "prs", "--limit", "nope")
	if code != 2 {
		t.Fatalf("code = %d, want 2 for a non-numeric limit: %s", code, out)
	}
	if !strings.Contains(out, "--limit wants a positive number") {
		t.Errorf("should name what --limit rejected: %q", out)
	}
}

// TestPRListWithNoPRsSaysSo is the control: an empty project must not error, and must say plainly
// that there is nothing rather than printing a bare, unexplained footer.
func TestPRListWithNoPRsSaysSo(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	if err := ps.PutAgent(store.Agent{Name: "eitri", Role: "worker", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}

	out, code := execAs(t, h, "eitri", "prs")
	if code != 0 || strings.TrimSpace(out) != "no PRs" {
		t.Errorf("code=%d out=%q, want a plain \"no PRs\"", code, out)
	}
}
