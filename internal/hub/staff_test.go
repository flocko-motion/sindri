package hub

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// staffFixture is a repo with a planner asking, two colleagues at work, and one in another project
// — the boundary this listing must not cross.
func staffFixture(t *testing.T) *Hub {
	t.Helper()
	h := newHub(t)
	ps := h.store.For(testProject)
	for _, a := range []store.Agent{
		{Name: "galar", Role: "planner"},
		{Name: "nori", Role: "worker"},
		{Name: "dvalin", Role: "reviewer"},
		{Name: "bombur", Role: "worker", Retired: true},
	} {
		if err := ps.PutAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.SetState(store.AgentState{Agent: "nori", Task: "sd-1", Phase: "working"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "sd-1", Agent: "nori", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	rev, err := ps.AddReview("pr-1", "check it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(rev, "dvalin"); err != nil {
		t.Fatal(err)
	}
	if err := h.store.For("other-repo").PutAgent(store.Agent{Name: "dwalin", Role: "planner"}); err != nil {
		t.Fatal(err)
	}
	return h
}

// staffOf runs the verb as agent and returns what it printed.
func staffOf(t *testing.T, h *Hub, agent string) string {
	t.Helper()
	var out bytes.Buffer
	if _, err := h.AgentExec(testProject, agent, []string{"staff"}, &out); err != nil {
		t.Fatalf("staff: %v", err)
	}
	return out.String()
}

// TestStaffNamesTheRepoColleagues is what the verb is for: a planner plans for people, and could
// not see who they were. Name, role, and what each holds — enough to decide who to ask.
func TestStaffNamesTheRepoColleagues(t *testing.T) {
	h := staffFixture(t)
	got := staffOf(t, h, "galar")
	for _, want := range []string{"nori", "worker", "sd-1", "dvalin", "reviewer", "pr-1"} {
		if !strings.Contains(got, want) {
			t.Errorf("staff listing missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "galar") || !strings.Contains(got, "(you)") {
		t.Errorf("the caller should recognise itself in the listing:\n%s", got)
	}
}

// TestStaffStopsAtTheRepoBoundary is the rule the epic turns on: visibility is local while
// addressing is global. A name from another repo reaches an agent through the user telling it,
// never through a lookup — so this must not become a way to go browsing the fleet.
func TestStaffStopsAtTheRepoBoundary(t *testing.T) {
	h := staffFixture(t)
	if got := staffOf(t, h, "galar"); strings.Contains(got, "dwalin") {
		t.Errorf("an agent of another project is listed — this is not a fleet directory:\n%s", got)
	}
}

// TestStaffSaysWhoIsFreeAndWhoIsWindingDown: "who do I ask" is half the question and "will they
// take it" is the other, so an idle colleague and a retired one read differently.
func TestStaffSaysWhoIsFreeAndWhoIsWindingDown(t *testing.T) {
	h := staffFixture(t)
	for _, line := range strings.Split(strings.TrimSpace(staffOf(t, h, "galar")), "\n") {
		switch {
		case strings.HasPrefix(line, "bombur"):
			if !strings.Contains(line, "retired") {
				t.Errorf("a retired colleague must say so — it takes nothing new: %q", line)
			}
		case strings.HasPrefix(line, "galar"):
			if !strings.Contains(line, "nothing in hand") {
				t.Errorf("an agent holding nothing should say so rather than trail off: %q", line)
			}
		}
	}
}

// TestStaffIsAPlannersVerb: a worker sees its own task and a reviewer its own PR, so neither is
// offered a listing of colleagues — the surface stays as narrow as the role's work.
func TestStaffIsAPlannersVerb(t *testing.T) {
	h := staffFixture(t)
	for agent, want := range map[string]bool{"galar": true, "nori": false, "dvalin": false} {
		var offered bool
		cmds, err := h.AgentCommands(testProject, agent)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cmds {
			if c.Name == "staff" {
				offered = true
			}
		}
		if offered != want {
			t.Errorf("%s offered staff = %v, want %v", agent, offered, want)
		}
	}
}
