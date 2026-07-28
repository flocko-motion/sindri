package td

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/task"
)

// tdInit creates a throwaway td store with the given tasks (each: title, then
// flag/value pairs) and returns its root. Skips if the td CLI isn't installed.
func tdStore(t *testing.T, creates [][]string) string {
	t.Helper()
	if _, err := exec.LookPath("td"); err != nil {
		t.Skip("td CLI not installed")
	}
	root := t.TempDir()
	if out, err := exec.Command("td", "-w", root, "init").CombinedOutput(); err != nil {
		t.Fatalf("td init: %s", out)
	}
	for _, c := range creates {
		args := append([]string{"-w", root, "create"}, c...)
		if out, err := exec.Command("td", args...).CombinedOutput(); err != nil {
			t.Fatalf("td create %v: %s", c, out)
		}
	}
	return root
}

func find(tasks []task.Task, titlePrefix string) (task.Task, bool) {
	for _, t := range tasks {
		if len(t.Title) >= len(titlePrefix) && t.Title[:len(titlePrefix)] == titlePrefix {
			return t, true
		}
	}
	return task.Task{}, false
}

// The direct DB reader must match what the CLI would return: fields, labels,
// and the filter (open vs all).
func TestTasksFromDBMatchesCLI(t *testing.T) {
	root := tdStore(t, [][]string{
		{"-t", "feature", "-p", "high", "--labels", "spec:add-auth,require-review-code", "Wire the authentication thing"},
		{"-t", "bug", "-p", "low", "Fix the annoying glitch bug"},
	})

	open, err := tasksFromDB(root, task.FilterOpen)
	if err != nil {
		t.Fatalf("tasksFromDB: %v", err)
	}
	if len(open) != 2 {
		t.Fatalf("want 2 open tasks, got %d: %+v", len(open), open)
	}

	auth, ok := find(open, "Wire the authentication")
	if !ok {
		t.Fatalf("auth task missing: %+v", open)
	}
	if auth.Type != "feature" {
		t.Errorf("type: got %q", auth.Type)
	}
	if auth.Status != "open" {
		t.Errorf("status: got %q", auth.Status)
	}
	if len(auth.Labels) != 2 || auth.Labels[0] != "spec:add-auth" || auth.Labels[1] != "require-review-code" {
		t.Errorf("labels parsed wrong: %#v", auth.Labels)
	}
	if !strings.HasPrefix(auth.ID, "td-") {
		t.Errorf("id: got %q", auth.ID)
	}

	// The bug task has no labels → empty slice, not a [""].
	bug, ok := find(open, "Fix the annoying")
	if !ok {
		t.Fatalf("bug task missing")
	}
	if len(bug.Labels) != 0 {
		t.Errorf("expected no labels, got %#v", bug.Labels)
	}

	// taskFromDB returns the same single task.
	got, err := taskFromDB(root, auth.ID)
	if err != nil {
		t.Fatalf("taskFromDB: %v", err)
	}
	if got.ID != auth.ID || got.Title != auth.Title {
		t.Fatalf("taskFromDB mismatch: %+v vs %+v", got, auth)
	}
}

// TestTasksFromDBCarriesDescription guards the fix: td stores a task's body in the
// issues table's `description` column, but the read's column list omitted it — so every
// td task reached the hub with an empty Description and the TUI's detail pane had nothing
// to show. The board's whole point is reading a task without opening td.
func TestTasksFromDBCarriesDescription(t *testing.T) {
	const body = "Steps:\n1. reproduce\n2. fix"
	root := tdStore(t, [][]string{
		{"-t", "bug", "-d", body, "Crash on empty input"},
		{"-t", "task", "No body on this one"},
	})

	tasks, err := tasksFromDB(root, task.FilterOpen)
	if err != nil {
		t.Fatalf("tasksFromDB: %v", err)
	}
	got, ok := find(tasks, "Crash on empty input")
	if !ok {
		t.Fatalf("task not found in %v", tasks)
	}
	if got.Description != body {
		t.Errorf("description = %q, want %q", got.Description, body)
	}
	// A task with no body must read as empty, not as a scan failure or a stray value.
	if none, ok := find(tasks, "No body on this one"); !ok || none.Description != "" {
		t.Errorf("bodyless task should have an empty description, got %q", none.Description)
	}
	// The single-task read shares dbCols, so it must agree with the list read.
	one, err := taskFromDB(root, got.ID)
	if err != nil {
		t.Fatalf("taskFromDB: %v", err)
	}
	if one.Description != body {
		t.Errorf("single read description = %q, want %q", one.Description, body)
	}
}
