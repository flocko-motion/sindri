package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// thread is one comment per source, so a test can tell apart "rendered" from "rendered the one
// field I happened to look for".
var thread = []store.Comment{
	{Source: "github", Author: "dain", Body: "Repro on 1.26 too.", CreatedAt: "2026-08-10T09:30:00Z"},
	{Source: "td", Author: "bombur", Body: "Root cause is the cache key.\nSecond line.", CreatedAt: "2026-08-11T14:05:00Z"},
}

// wantsThread asserts every field the thread must carry. Author, body, time and SOURCE — the
// source says who else has already seen the comment, which is what a reply is written for.
func wantsThread(t *testing.T, got string) {
	t.Helper()
	for _, want := range []string{
		"dain", "Repro on 1.26 too.", "github", "2026-08-10T09:30:00Z",
		"bombur", "Root cause is the cache key.", "Second line.", "td", "2026-08-11T14:05:00Z",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the thread is missing %q:\n%s", want, got)
		}
	}
	// Oldest first, matching the TUI: a thread read bottom-up puts the answer before the question.
	if i, j := strings.Index(got, "dain"), strings.Index(got, "bombur"); i > j {
		t.Errorf("comments are newest-first; the thread should read oldest-first:\n%s", got)
	}
}

// TestBareTaskShowsTheThread is the case the ticket was filed from: an agent writes a finding on
// its task and goes to read it back. Bare `task` is where it looks first, and that view reads the
// task ROW, whose table holds no comments — so this needed a fetch, not just a render.
func TestBareTaskShowsTheThread(t *testing.T) {
	e, c := workerEngineComments(t, []store.Task{
		{ID: "td-9", Title: "Fix the glitch", Status: "open", Priority: "P1", Description: "Steps: reproduce, then fix."},
	}, "", "td-9", map[string][]store.Comment{"td-9": thread})

	var out bytes.Buffer
	tasks, err := e.store.For("proj").AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.workerTaskView(c, tasks, &out); err != nil {
		t.Fatalf("workerTaskView: %v", err)
	}
	wantsThread(t, out.String())
}

// TestBareTaskOnAPackageShowsThePackageThread: a comment on a package is addressed to whoever
// holds it, which is the reader of this view. The subtask listing stays a listing — each subtask's
// own thread is one `task <id>` away.
func TestBareTaskOnAPackageShowsThePackageThread(t *testing.T) {
	e, c := workerEngineComments(t, []store.Task{
		{ID: "td-EPIC", Title: "Login feature", Status: "open", Priority: "P1", Description: "Users must sign in."},
		{ID: "td-1", Title: "Form UI", Status: "open", Priority: "P1", ParentID: "td-EPIC"},
		{ID: "td-2", Title: "Session store", Status: "open", Priority: "P1", ParentID: "td-EPIC"},
	}, "td-EPIC", "td-2", map[string][]store.Comment{"td-EPIC": thread})

	var out bytes.Buffer
	tasks, err := e.store.For("proj").AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.workerTaskView(c, tasks, &out); err != nil {
		t.Fatalf("workerTaskView: %v", err)
	}
	got := out.String()
	wantsThread(t, got)
	if !strings.Contains(got, "subtasks") {
		t.Errorf("the package overview lost its subtask listing:\n%s", got)
	}
}

// ownedTaskEngine seeds a task sindri OWNS, which is the case the ticket is about: an agent
// comments on its own task. Owned rows are the source the task cache is rebuilt from, so they
// survive the sync that `task <id>` runs first — a cache-only row would be swept before it is read.
func ownedTaskEngine(t *testing.T, id, title string, comments []store.Comment) (*Engine, registry.Caller) {
	t.Helper()
	e, c := workerEngineComments(t, nil, "", id, map[string][]store.Comment{id: comments})
	if err := e.store.For("proj").PutOwnedTask(store.OwnedTask{
		ID: id, Title: title, Status: "in_progress", Priority: "P1", Description: "Reported.",
	}); err != nil {
		t.Fatalf("put owned task: %v", err)
	}
	return e, c
}

// TestTaskByIDShowsTheThread covers the fuller view. TaskInfo already attached the comments and
// put them on the wire; only the rendering was missing, so an agent was handed a task carrying its
// thread and shown everything except.
func TestTaskByIDShowsTheThread(t *testing.T) {
	e, c := ownedTaskEngine(t, "sd-a1b2c3", "Own task", thread)

	var out bytes.Buffer
	if _, err := e.CmdTasks(c, []string{"sd-a1b2c3"}, &out); err != nil {
		t.Fatalf("CmdTasks: %v", err)
	}
	wantsThread(t, out.String())
}

// TestPlannerSeesTheThreadToo: the planner reads the same view unbounded, and a planner that
// cannot see the discussion re-decides questions the thread already settled.
func TestPlannerSeesTheThreadToo(t *testing.T) {
	e, _ := ownedTaskEngine(t, "sd-a1b2c3", "Own task", thread)
	planner := registry.Caller{Project: "proj", Agent: "brokk", Role: "planner"}

	var out bytes.Buffer
	if _, err := e.CmdTasks(planner, []string{"sd-a1b2c3"}, &out); err != nil {
		t.Fatalf("CmdTasks: %v", err)
	}
	wantsThread(t, out.String())
}

// TestNoCommentsPrintsNoHeading: a task nobody has commented on must not grow a "comments (0)"
// line. The view is re-read constantly by agents, and a heading that is always there for an empty
// thing is the kind of noise that teaches skimming.
func TestNoCommentsPrintsNoHeading(t *testing.T) {
	e, c := workerEngine(t, []store.Task{
		{ID: "td-9", Title: "Fix the glitch", Status: "open", Priority: "P1", Description: "Steps."},
	}, "", "td-9")

	var out bytes.Buffer
	tasks, err := e.store.For("proj").AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.workerTaskView(c, tasks, &out); err != nil {
		t.Fatalf("workerTaskView: %v", err)
	}
	if got := out.String(); strings.Contains(got, "comment") || strings.Contains(got, "—") {
		t.Errorf("an empty thread should render nothing at all:\n%s", got)
	}
}
