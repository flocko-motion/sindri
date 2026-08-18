package workflow

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// backlogEngine seeds the shape the complaint came from: a handful of open tasks under a pile of
// closed history, and an open subtask whose parent is closed.
//
// The tasks carry no useful age: PutOwnedTask stamps updated_at as it writes, so a fixture cannot
// back-date one, and every row here reads as changed just now. So these tests prove the verb APPLIES
// a filter and refuses a bad one; that `active` means "open, plus closed inside ActiveWindow" is
// api.MatchesFilter's own rule, tested over a dated backlog in internal/api.
func backlogEngine(t *testing.T) (*Engine, registry.Caller) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("proj")
	tasks := []store.OwnedTask{
		{ID: "sd-open1", Title: "open work", Status: "open", Priority: "P1"},
		{ID: "sd-open2", Title: "more open work", Status: "open", Priority: "P2"},
		{ID: "sd-done1", Title: "closed history", Status: "closed", Priority: "P2"},
		{ID: "sd-done2", Title: "more closed history", Status: "merged", Priority: "P2"},
		{ID: "sd-done3", Title: "closed history still", Status: "approved", Priority: "P3"},
		// The tree case: an open subtask whose parent is closed.
		{ID: "sd-parent", Title: "a closed feature", Status: "closed", Priority: "P1"},
		{ID: "sd-child", Title: "its unfinished subtask", Status: "open", Priority: "P2"},
	}
	for _, task := range tasks {
		if err := ps.PutOwnedTask(task); err != nil {
			t.Fatal(err)
		}
		if err := ps.UpsertTask(store.Task{ID: task.ID, Title: task.Title, Status: task.Status,
			Priority: task.Priority}); err != nil {
			t.Fatal(err)
		}
	}
	if err := ps.SetParent("sd-child", "sd-parent"); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutAgent(store.Agent{Name: "nabbi", Role: "planner", Workspace: ".worktrees/nabbi"}); err != nil {
		t.Fatal(err)
	}
	return New(st, &stubDeps{root: root}), registry.Caller{Project: "proj", Agent: "nabbi", Role: "planner"}
}

// listTasks runs `task list` with the given arguments and returns its output and exit code.
func listTasks(t *testing.T, e *Engine, c registry.Caller, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code, err := e.CmdTasks(c, append([]string{"list"}, args...), &out)
	if err != nil {
		t.Fatalf("CmdTasks %v: %v", args, err)
	}
	return out.String(), code
}

// TestTheDefaultIsActive is the day-to-day complaint: a planner asked for its work and got a hundred
// and seventy rows of closed history. Active is what it plans against — open work, plus whatever has
// only just stopped being open, so work finished a moment ago has not already vanished.
func TestTheDefaultIsActive(t *testing.T) {
	e, c := backlogEngine(t)
	bare, code := listTasks(t, e, c)
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, bare)
	}
	asked, _ := listTasks(t, e, c, "--filter", string(api.FilterActive))
	if bare != asked {
		t.Errorf("the bare listing should be the active one:\n%s\n---\n%s", bare, asked)
	}
	// And it says which filter it applied, so a reader never has to guess why a row is missing.
	if !strings.Contains(bare, string(api.FilterActive)) {
		t.Errorf("the listing should name the filter it used:\n%s", bare)
	}
	// That it is not `all` cannot be shown from these rows — every one of them reads as changed just
	// now, so active admits the lot (-> backlogEngine). The equality above is the whole claim: the
	// default IS the active filter, and what that filter admits is api.MatchesFilter's own tested rule.
}

// TestTheNarrowingSaysWhatItHid is what makes a narrow default safe. Fourteen rows out of a hundred
// and seventy-seven must never read as a backlog of fourteen, so the listing closes by stating both.
func TestTheNarrowingSaysWhatItHid(t *testing.T) {
	e, c := backlogEngine(t)
	got, _ := listTasks(t, e, c, "--filter", "open")
	// Three open tasks in a backlog of seven, four of them closed.
	want := api.TaskListSummary(api.FilterOpen, 3, tasksOf(t, e, c))
	if !strings.Contains(got, want) {
		t.Errorf("the listing should close with %q:\n%s", want, got)
	}
	for _, part := range []string{"showing 3 open", "4 closed in total"} {
		if !strings.Contains(got, part) {
			t.Errorf("the summary should say %q:\n%s", part, got)
		}
	}
}

// tasksOf is the backlog as the engine sees it, for building the expected summary.
func tasksOf(t *testing.T, e *Engine, c registry.Caller) []store.Task {
	t.Helper()
	tasks, err := e.store.For(c.Project).AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	return tasks
}

// TestEveryFilterNarrowsDifferently is the check that was impossible before: identical output under
// every filter is exactly what read as a broken filter and sent the reader into the wrong code path.
func TestEveryFilterNarrowsDifferently(t *testing.T) {
	e, c := backlogEngine(t)
	seen := map[string]string{}
	for _, f := range api.TaskFilters {
		got, code := listTasks(t, e, c, "--filter", string(f))
		if code != 0 {
			t.Fatalf("--filter %s: exit %d:\n%s", f, code, got)
		}
		seen[string(f)] = got
	}
	if !strings.Contains(seen["all"], "sd-done1") {
		t.Errorf("`all` should hold the closed history:\n%s", seen["all"])
	}
	if strings.Contains(seen["open"], "sd-done1") {
		t.Errorf("`open` should not hold the closed history:\n%s", seen["open"])
	}
	if !strings.Contains(seen["closed"], "sd-done1") || strings.Contains(seen["closed"], "sd-open1") {
		t.Errorf("`closed` should hold the done segment and nothing else:\n%s", seen["closed"])
	}
	if seen["all"] == seen["open"] || seen["open"] == seen["closed"] {
		t.Error("two filters produced identical output, which is the defect this fixes")
	}
}

// TestTheInlineFormWorksToo: `--filter=active` is how half of everyone writes it, and create-task
// already accepts both forms — a verb that took one and silently ignored the other would be the same
// defect in a new place.
func TestTheInlineFormWorksToo(t *testing.T) {
	e, c := backlogEngine(t)
	inline, code := listTasks(t, e, c, "--filter=closed")
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, inline)
	}
	spaced, _ := listTasks(t, e, c, "--filter", "closed")
	if inline != spaced {
		t.Errorf("the two spellings should mean the same thing:\n%s\n---\n%s", inline, spaced)
	}
}

// TestAnUnknownArgumentIsRefused is the defect that cost the investigation: every argument printed
// the same rows and said nothing, so the filter looked broken when it was simply absent. Silence is
// the failure — the listing must not appear at all.
func TestAnUnknownArgumentIsRefused(t *testing.T) {
	e, c := backlogEngine(t)
	for _, args := range [][]string{{"--wibble"}, {"--filter", "bogus"}, {"--filter"}, {"nonsense"}, {"-x", "1"}} {
		got, code := listTasks(t, e, c, args...)
		if code == 0 {
			t.Errorf("%v was accepted:\n%s", args, got)
		}
		if strings.Contains(got, "sd-open1") {
			t.Errorf("%v printed the listing anyway, which is what made it look like a working filter:\n%s", args, got)
		}
		// And it names what IS accepted, so the next attempt succeeds.
		if !strings.Contains(got, api.TaskFilterNames()) {
			t.Errorf("%v: the refusal should name the filters on offer:\n%s", args, got)
		}
	}
}

// TestAFlagWhereAnIdBelongsIsRefused: `task --wibble` used to be read as a task id, so a mistyped
// flag was answered with "no such task" — which sends the reader looking for a task rather than at
// what they typed.
func TestAFlagWhereAnIdBelongsIsRefused(t *testing.T) {
	e, c := backlogEngine(t)
	var out bytes.Buffer
	code, err := e.CmdTasks(c, []string{"--wibble"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 {
		t.Errorf("a flag where an id belongs was accepted:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "unknown argument") {
		t.Errorf("the refusal should say what was wrong:\n%s", out.String())
	}
}

// TestAnAncestorStaysVisibleAsContext: the listing is indented by tree, so an open subtask whose
// parent the filter dropped would be re-rooted and read as belonging to nobody — and which tree a
// task hangs in is most of what a planner reads this view for.
func TestAnAncestorStaysVisibleAsContext(t *testing.T) {
	e, c := backlogEngine(t)
	got, _ := listTasks(t, e, c, "--filter", "open") // the parent is closed, the child is not
	if !strings.Contains(got, "sd-parent") {
		t.Errorf("the closed parent of a shown subtask should stay visible:\n%s", got)
	}
	// Marked, so it is not mistaken for a match: it is closed, and the filter is active.
	parent := lineWith(got, "sd-parent")
	if !strings.Contains(parent, "context") {
		t.Errorf("a parent kept for context must say so, or it reads as a filter leak: %q", parent)
	}
	// And the child is still indented under it rather than re-rooted.
	child := lineWith(got, "sd-child")
	if !strings.Contains(child, "  its unfinished subtask") {
		t.Errorf("the subtask should keep its depth under the parent: %q", child)
	}
	// The summary accounts for the extra row, so the count and the rows on screen agree.
	if !strings.Contains(got, "parent task shown for context") {
		t.Errorf("the summary should account for the context row:\n%s", got)
	}
}

// lineWith is the first line of out containing s ("" if none).
func lineWith(out, s string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

// TestTheLenientValueRuleIsUntouched pins the distinction the fix rests on. api.MatchesFilter still
// admits everything on a filter value it does not know — a listing showing nothing would claim an
// empty backlog, which is a lie a wrong flag should not be able to tell. That is a lenient answer to
// a question that was understood; an unknown ARGUMENT is not answering at all, and is refused above.
func TestTheLenientValueRuleIsUntouched(t *testing.T) {
	if !api.MatchesFilter("nonsense", api.Task{Status: "closed"}) {
		t.Error("an unrecognised filter value must still admit everything")
	}
	if !api.MatchesFilter("nonsense", api.Task{Status: "open"}) {
		t.Error("an unrecognised filter value must still admit everything")
	}
}
