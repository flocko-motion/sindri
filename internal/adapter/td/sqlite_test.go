package td

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/issue"
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

func find(tasks []issue.Task, titlePrefix string) (issue.Task, bool) {
	for _, t := range tasks {
		if len(t.Title) >= len(titlePrefix) && t.Title[:len(titlePrefix)] == titlePrefix {
			return t, true
		}
	}
	return issue.Task{}, false
}

// The direct DB reader must match what the CLI would return: fields, labels,
// and the filter (open vs all).
func TestTasksFromDBMatchesCLI(t *testing.T) {
	root := tdStore(t, [][]string{
		{"-t", "feature", "-p", "high", "--labels", "spec:add-auth,require-review-code", "Wire the authentication thing"},
		{"-t", "bug", "-p", "low", "Fix the annoying glitch bug"},
	})

	open, err := tasksFromDB(root, issue.FilterOpen)
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
