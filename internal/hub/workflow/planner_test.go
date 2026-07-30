package workflow

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// plannerEngine builds an engine over a throwaway store with one cached task in the given
// approval state, and returns the caller a planner would arrive as.
func plannerEngine(t *testing.T, id, approval string) (*Engine, registry.Caller, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	if err := ps.UpsertTask(store.Task{ID: id, Title: "flat proposal", Status: "open"}); err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	if approval != "" {
		if err := ps.SetApproval(id, approval, ""); err != nil {
			t.Fatalf("set approval: %v", err)
		}
	}
	e := New(st, &stubDeps{root: t.TempDir()})
	return e, registry.Caller{Project: "proj", Agent: "galar", Role: "planner"}, ps
}

// TestEditTaskRefusedOnceApproved is the constraint the approval gate exists for: approval is
// the user's decision to take the task as it stands, so what a worker picks up is what the
// user read and released.
func TestEditTaskRefusedOnceApproved(t *testing.T) {
	for _, approval := range []string{"approved", "rejected", ""} {
		e, c, _ := plannerEngine(t, "td-1", approval)
		var out bytes.Buffer
		code, err := e.CmdEditTask(c, []string{"td-1", "--parent", "td-9"}, &out)
		if err != nil {
			t.Fatalf("approval %q: unexpected error: %v", approval, err)
		}
		if code == 0 {
			t.Errorf("approval %q: edit should be refused, got exit 0: %s", approval, out.String())
		}
		if !strings.Contains(out.String(), "awaiting the user's approval") {
			t.Errorf("approval %q: refusal should state the rule, got: %s", approval, out.String())
		}
	}
}

// TestEditTaskUsageIsVisible: a malformed call answers with usage on the agent's own stream.
// A returned error would reach it as "the hub hit an internal error", which it cannot act on.
func TestEditTaskUsageIsVisible(t *testing.T) {
	e, c, _ := plannerEngine(t, "td-1", "pending")
	for _, args := range [][]string{{}, {"td-1"}, {"td-1", "--nope", "x"}} {
		var out bytes.Buffer
		code, err := e.CmdEditTask(c, args, &out)
		if err != nil {
			t.Fatalf("args %v: usage must not be an error: %v", args, err)
		}
		if code != 2 {
			t.Errorf("args %v: exit = %d, want 2", args, code)
		}
		if !strings.Contains(out.String(), "usage: edit-task") {
			t.Errorf("args %v: expected usage, got: %s", args, out.String())
		}
	}
}

// TestPlannerCannotSetPriority: OpenLeaves hands out only tasks that are approved AND carry a
// priority, so the priority a human sets at approval is what releases work. A planner setting
// it would hand itself the release switch.
func TestPlannerCannotSetPriority(t *testing.T) {
	for _, flag := range []string{"--priority", "-p"} {
		if _, _, err := parseTaskFlags([]string{flag, "P0", "urgent thing"}); err == nil {
			t.Errorf("%s should be refused, not accepted", flag)
		}
	}
	if strings.Contains(createTaskUsage, "--priority") {
		t.Error("create-task usage should not advertise --priority")
	}
}

// TestUnknownFlagRefused: a silently ignored flag looks like it took effect, and the task is
// then created or edited without the parent or body that was asked for.
func TestUnknownFlagRefused(t *testing.T) {
	if _, _, err := parseTaskFlags([]string{"--tree"}); err == nil {
		t.Error("an unknown flag should be an error the caller sees")
	}
	if _, _, err := parseTaskFlags([]string{"--parent"}); err == nil {
		t.Error("a flag with no value should be an error")
	}
}

// TestParseTaskFlagsFormsAndTitle: both `--flag value` and `--flag=value`, with the leftover
// words forming the title.
func TestParseTaskFlagsFormsAndTitle(t *testing.T) {
	s, words, err := parseTaskFlags([]string{"--parent=os-adc678", "--type", "feature", "--body", "why it matters", "wire", "the", "thing"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Parent != "os-adc678" || s.Type != "feature" || s.Description != "why it matters" {
		t.Errorf("flags parsed as %+v", s)
	}
	if got := strings.Join(words, " "); got != "wire the thing" {
		t.Errorf("title words = %q", got)
	}
}

// TestSyncToleratesRepoWithoutTd: a repo that tracks work only in openspec or GitHub issues
// must still list those. td gates on having a store, so its absence leaves it with nothing to
// contribute instead of failing the sync for every source — which is what made the host CLI
// error with "no td store" while the TUI, reading the cache, happily showed the same repo's
// tasks. One source's absence must not decide the answer for all of them.
func TestSyncToleratesRepoWithoutTd(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir() // no .todos, no openspec/, no GitHub remote
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	e := New(st, &stubDeps{root: root})

	if err := e.SyncTasks("proj"); err != nil {
		t.Fatalf("a repo with no td store should sync cleanly, got: %v", err)
	}
	// And the read path the host CLI uses must agree, rather than surfacing an error the
	// board never sees.
	if _, err := e.Tasks("proj"); err != nil {
		t.Fatalf("Tasks should succeed with no td store, got: %v", err)
	}
}

// workerEngine seeds a worker holding `held` (a package when it has children) and returns the
// engine plus the caller a worker arrives as.
func workerEngine(t *testing.T, tasks []store.Task, container, current string) (*Engine, registry.Caller) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	for _, task := range tasks {
		if err := ps.UpsertTask(task); err != nil {
			t.Fatalf("upsert %s: %v", task.ID, err)
		}
	}
	if err := ps.SetState(store.AgentState{Agent: "eitri", Container: container, Task: current, Phase: "working"}); err != nil {
		t.Fatalf("set state: %v", err)
	}
	return New(st, &stubDeps{root: root}), registry.Caller{Project: "proj", Agent: "eitri", Role: "worker"}
}

// TestWorkerSeesItsPackage: a package is claimed whole for the context it carries, so that
// context must be re-readable — the claim directive alone loses it to a compaction or relaunch.
// The overview names the parent, its description, and every subtask, marking the current one.
func TestWorkerSeesItsPackage(t *testing.T) {
	e, c := workerEngine(t, []store.Task{
		{ID: "td-EPIC", Title: "Login feature", Status: "open", Priority: "P1", Description: "Users must sign in."},
		{ID: "td-1", Title: "Form UI", Status: "closed", Priority: "P1", ParentID: "td-EPIC"},
		{ID: "td-2", Title: "Session store", Status: "open", Priority: "P1", ParentID: "td-EPIC"},
		{ID: "td-3", Title: "Wire it up", Status: "open", Priority: "P2", ParentID: "td-2"}, // nested
		{ID: "td-OTHER", Title: "Someone else's", Status: "open", Priority: "P1"},
	}, "td-EPIC", "td-2")

	// The view directly: CmdTasks would first sync, and a temp repo has no task source, so
	// the cache would be replaced with nothing before the view ran.
	var out bytes.Buffer
	tasks, err := e.store.For("proj").AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if code, err := e.workerTaskView(c, tasks, &out); err != nil || code != 0 {
		t.Fatalf("workerTaskView: code=%d err=%v", code, err)
	}
	got := out.String()
	for _, want := range []string{
		"td-EPIC", "Login feature", "Users must sign in.", // the parent and its body
		"td-1", "td-2", "td-3", // every descendant, including the nested one
		"→ td-2",             // the subtask it is on
		"`sindri task <id>`", // how to read one in full
	} {
		if !strings.Contains(got, want) {
			t.Errorf("overview missing %q:\n%s", want, got)
		}
	}
	// The overview is its OWN package, not the backlog.
	if strings.Contains(got, "td-OTHER") {
		t.Errorf("a worker's view must not list other work:\n%s", got)
	}
}

// TestWorkerStandaloneTaskShowsInFull: with nothing to choose between, asking the worker to
// pick would be a wasted round trip.
func TestWorkerStandaloneTaskShowsInFull(t *testing.T) {
	e, c := workerEngine(t, []store.Task{
		{ID: "td-9", Title: "Fix the glitch", Status: "open", Priority: "P1", Description: "Steps: reproduce, then fix."},
	}, "", "td-9")

	var out bytes.Buffer
	tasks, err := e.store.For("proj").AllTasks()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.workerTaskView(c, tasks, &out); err != nil {
		t.Fatalf("workerTaskView: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Steps: reproduce, then fix.") {
		t.Errorf("a standalone task should print its description:\n%s", got)
	}
	if strings.Contains(got, "subtasks") {
		t.Errorf("a standalone task has no subtask overview:\n%s", got)
	}
}

// TestWorkerWithNoTaskIsToldWhatToDo: an idle worker gets the one next step, not an empty view.
func TestWorkerWithNoTaskIsToldWhatToDo(t *testing.T) {
	e, c := workerEngine(t, nil, "", "")
	var out bytes.Buffer
	if _, err := e.CmdTasks(c, nil, &out); err != nil {
		t.Fatalf("CmdTasks: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "no task") || !strings.Contains(got, "`sindri`") {
		t.Errorf("expected a pointer to picking work up, got: %q", got)
	}
}

// visibleFor is the boundary under test: which tasks a caller may read, and whether that set is
// bounded at all. Exercised directly rather than through CmdTasks, which syncs first — and a
// temp repo has no task source, so the sync would replace the seeded cache with nothing.
func visibleFor(t *testing.T, e *Engine, c registry.Caller, tasks []store.Task) (map[string]bool, bool) {
	t.Helper()
	v, bounded, err := e.visibleTasks(c, tasks)
	if err != nil {
		t.Fatal(err)
	}
	return v, bounded
}

// TestWorkerIsBoundedToItsPackage: a worker's job is to finish one piece of work, so the rest of
// the backlog is at best a distraction and at worst an invitation to start something nobody
// assigned it. Its package — the held task and every descendant — is what it may read.
func TestWorkerIsBoundedToItsPackage(t *testing.T) {
	tasks := []store.Task{
		{ID: "td-EPIC", Title: "Login feature", Status: "open", Priority: "P1"},
		{ID: "td-1", Title: "Form UI", Status: "open", Priority: "P1", ParentID: "td-EPIC"},
		{ID: "td-2", Title: "Session store", Status: "open", Priority: "P1", ParentID: "td-1"}, // nested
		{ID: "td-OTHER", Title: "Someone else's epic", Status: "open", Priority: "P0"},
		{ID: "td-OC", Title: "…and its child", Status: "open", Priority: "P0", ParentID: "td-OTHER"},
	}
	e, c := workerEngine(t, tasks, "td-EPIC", "td-1")

	visible, bounded := visibleFor(t, e, c, tasks)
	if !bounded {
		t.Fatal("a worker's view must be bounded")
	}
	for _, want := range []string{"td-EPIC", "td-1", "td-2"} { // the package, descendants included
		if !visible[want] {
			t.Errorf("%s is part of the worker's package and must be visible", want)
		}
	}
	for _, hidden := range []string{"td-OTHER", "td-OC"} {
		if visible[hidden] {
			t.Errorf("%s is not the worker's work and must be hidden", hidden)
		}
	}
}

// TestWorkerWithoutTaskSeesNothing: an idle worker has no package, so it reads none of the
// backlog — the bound is on what it HOLDS, not on having asked politely.
func TestWorkerWithoutTaskSeesNothing(t *testing.T) {
	tasks := []store.Task{{ID: "td-A", Title: "Alpha", Status: "open", Priority: "P1"}}
	e, c := workerEngine(t, tasks, "", "")
	visible, bounded := visibleFor(t, e, c, tasks)
	if !bounded || len(visible) != 0 {
		t.Errorf("an idle worker should see nothing, got bounded=%v %v", bounded, visible)
	}
}

// TestPlannerAndCoauthorAreUnbounded: they shape the backlog across packages, and a coauthor
// works directly with the user, so both need the picture the user has.
func TestPlannerAndCoauthorAreUnbounded(t *testing.T) {
	tasks := []store.Task{{ID: "td-A", Status: "open"}, {ID: "td-B", Status: "open"}}
	for _, role := range []string{"planner", "coauthor"} {
		e, c := workerEngine(t, tasks, "", "")
		c.Role = role
		if _, bounded := visibleFor(t, e, c, tasks); bounded {
			t.Errorf("%s must see the whole backlog", role)
		}
	}
}

// TestWorkerIdRefusalNamesTheWayBack: bounding the listing would be theatre if any id could be
// read directly, so the same boundary governs `task <id>` — and the refusal points at the view
// the worker actually wanted, without leaking the task it refused.
func TestWorkerIdRefusalNamesTheWayBack(t *testing.T) {
	tasks := []store.Task{
		{ID: "td-MINE", Title: "Mine", Status: "open", Priority: "P1"},
		{ID: "td-THEIRS", Title: "Theirs", Status: "open", Priority: "P1", Description: "secret plan"},
	}
	e, c := workerEngine(t, tasks, "", "td-MINE")

	var out bytes.Buffer
	code, err := e.CmdTasks(c, []string{"td-THEIRS"}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code == 0 {
		t.Errorf("reading another agent's task must be refused, got exit 0:\n%s", out.String())
	}
	if strings.Contains(out.String(), "secret plan") {
		t.Errorf("the refusal leaked the task body:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "`sindri task`") {
		t.Errorf("the refusal should point at the worker's own package:\n%s", out.String())
	}
}
