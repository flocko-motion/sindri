package workflow

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// holderFixture is a backlog with one task being worked, one whose worker has submitted and moved
// to review, and one nobody has touched.
func holderFixture(t *testing.T) *Engine {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	// Owned rows AND the cache: the sync CmdTasks runs first rebuilds the cache from the owned
	// table, so a cache-only seed is swept away before the listing is drawn.
	for _, x := range []store.OwnedTask{
		{ID: "sd-worked", Title: "in hand", Status: "open", Priority: "P1"},
		{ID: "sd-review", Title: "submitted", Status: "in_progress", Priority: "P1"},
		{ID: "sd-free", Title: "untouched", Status: "open", Priority: "P2"},
	} {
		if err := ps.PutOwnedTask(x); err != nil {
			t.Fatal(err)
		}
		if err := ps.UpsertTask(store.Task{ID: x.ID, Title: x.Title, Status: x.Status, Priority: x.Priority}); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []store.Agent{
		{Name: "galar", Role: "planner"},
		{Name: "nori", Role: "worker"},
	} {
		if err := ps.PutAgent(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.SetState(store.AgentState{Agent: "nori", Task: "sd-worked", Phase: "working"}); err != nil {
		t.Fatal(err)
	}
	// bombur submitted and is no longer holding the task — the PR is the only trace of who did it.
	if err := ps.PutPR(store.PR{ID: "pr-sd-review", Task: "sd-review", Agent: "bombur", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	return New(st, &stubDeps{root: root})
}

// taskView runs the agent-facing task verb as role and returns what it printed.
func taskView(t *testing.T, e *Engine, role string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	c := registry.Caller{Project: "repo", Agent: "galar", Role: role}
	if _, err := e.CmdTasks(c, args, &out); err != nil {
		t.Fatalf("task %v: %v", args, err)
	}
	return out.String()
}

// lineFor returns the listing line mentioning id.
func lineFor(out, id string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, id) {
			return l
		}
	}
	return ""
}

// TestTheListingNamesWhoHoldsEachTask is the gap: a planner reads the whole backlog and could not
// see who was working any of it, so it planned around slots rather than people.
func TestTheListingNamesWhoHoldsEachTask(t *testing.T) {
	out := taskView(t, holderFixture(t), "planner", "list")
	if got := lineFor(out, "sd-worked"); !strings.Contains(got, "nori") {
		t.Errorf("the worked task should name its holder: %q", got)
	}
	if got := lineFor(out, "sd-free"); strings.Contains(got, "nori") {
		t.Errorf("a task nobody holds must not borrow a name: %q", got)
	}
}

// TestASubmittedTaskStillNamesItsAuthor: the work is done and the question is who to ask about it,
// which is exactly when the holder had disappeared from the view — the agent has moved on and only
// the PR remembers.
func TestASubmittedTaskStillNamesItsAuthor(t *testing.T) {
	e := holderFixture(t)
	if got := lineFor(taskView(t, e, "planner", "list"), "sd-review"); !strings.Contains(got, "bombur") {
		t.Errorf("a task under review should name who submitted it: %q", got)
	}
	if detail := taskView(t, e, "planner", "sd-review"); !strings.Contains(detail, "bombur") {
		t.Errorf("its detail should too:\n%s", detail)
	}
}

// TestTheDetailNamesTheHolder pins the second of the two views the subtask names, since a planner
// reading one task in full is the other half of the same question.
func TestTheDetailNamesTheHolder(t *testing.T) {
	detail := taskView(t, holderFixture(t), "planner", "sd-worked")
	if !strings.Contains(detail, "agent:") || !strings.Contains(detail, "nori") {
		t.Errorf("the detail must carry the holder:\n%s", detail)
	}
}

// TestAWorkerIsNotGivenTheHolderColumn: a worker sees its own task and needs no directory, and a
// reviewer learns the author from the directive that hands it the PR (-> DirReview). Keeping the
// column to the roles that plan around people is what stops a task view becoming one.
func TestAWorkerIsNotGivenTheHolderColumn(t *testing.T) {
	e := holderFixture(t)
	if out := taskView(t, e, "reviewer", "list"); strings.Contains(lineFor(out, "sd-worked"), "nori") {
		t.Errorf("a reviewer's listing should not carry the holder column:\n%s", out)
	}
	if out := taskView(t, e, "coauthor", "list"); !strings.Contains(lineFor(out, "sd-worked"), "nori") {
		t.Errorf("a coauthor works alongside people and should see it:\n%s", out)
	}
}
