package workflow

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/tasks/td"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// tdRepo makes a repo with a td store, or skips: td is the primary backend, and these cases are
// about what td holds rather than what the cache remembers.
func tdRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("td"); err != nil {
		t.Skip("td not installed")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s", out)
	}
	if out, err := exec.Command("td", "-w", root, "init").CombinedOutput(); err != nil {
		t.Skipf("td init: %s", out)
	}
	return root
}

// tdCreate adds a task and returns its id.
func tdCreate(t *testing.T, root, title string) string {
	t.Helper()
	if out, err := exec.Command("td", "-w", root, "create", title).CombinedOutput(); err != nil {
		t.Fatalf("td create: %s", out)
	}
	tasks, err := td.Tasks(root, task.FilterAll)
	if err != nil || len(tasks) == 0 {
		t.Fatalf("no task after create (err %v)", err)
	}
	return tasks[len(tasks)-1].ID
}

// TestReconcileKeepsATaskTdSaysIsClosed is the merge bug: closing a task leaves the cached row at
// its old "in_progress" for a moment, and the sweep used to read that cache, see no PR and no
// assignee, and write "open" back to td — reopening work that had just merged, which the worker
// then re-claimed seconds later.
func TestReconcileKeepsATaskTdSaysIsClosed(t *testing.T) {
	root := tdRepo(t)
	id := tdCreate(t, root, "a task title long enough for td to accept it")

	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")

	// td holds the truth: the task is closed, exactly as a merge leaves it.
	if err := td.SetStatus(root, id, "closed"); err != nil {
		t.Fatalf("close in td: %v", err)
	}
	// The cache lags, still carrying the status from before the close.
	if err := ps.UpsertTask(store.Task{ID: id, Title: "t", Status: "in_progress", Priority: "P2"}); err != nil {
		t.Fatalf("seed stale cache: %v", err)
	}

	e := New(st, &stubDeps{root: root})
	if err := e.ReconcileTasks("proj"); err != nil {
		t.Fatalf("ReconcileTasks: %v", err)
	}
	live, err := td.Get(root, id)
	if err != nil {
		t.Fatalf("re-read %s: %v", id, err)
	}
	if live.Status != "closed" {
		t.Errorf("td says %q after the sweep, want \"closed\" — a stale cached row must not reopen finished work", live.Status)
	}
}

// TestReconcileStillRepairsAGenuinelyStaleTask: the repair itself has to keep working, or the fix
// above would just disable it. td says in_progress, nothing holds it, so open is correct.
func TestReconcileStillRepairsAGenuinelyStaleTask(t *testing.T) {
	root := tdRepo(t)
	id := tdCreate(t, root, "another task title long enough for td")

	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := td.SetStatus(root, id, "in_progress"); err != nil {
		t.Fatalf("set in_progress: %v", err)
	}

	e := New(st, &stubDeps{root: root})
	if err := e.ReconcileTasks("proj"); err != nil {
		t.Fatalf("ReconcileTasks: %v", err)
	}
	live, err := td.Get(root, id)
	if err != nil {
		t.Fatalf("re-read %s: %v", id, err)
	}
	if live.Status != "open" {
		t.Errorf("td says %q, want \"open\" — in_progress with no assignee is stale", live.Status)
	}
}

// TestReconciledStatusRule pins the pure rule the sweep applies.
func TestReconciledStatusRule(t *testing.T) {
	for _, c := range []struct {
		status            string
		activePR, assigne bool
		want              string
	}{
		{"in_progress", false, false, "open"},       // nobody holds it
		{"in_progress", false, true, "in_progress"}, // its worker still does
		{"in_review", false, false, "open"},         // no PR, nobody holds it
		{"in_review", false, true, "in_progress"},   // no PR, but still assigned
		{"in_review", true, false, "in_review"},     // a live PR justifies it
		{"closed", false, false, "closed"},          // a finished task is left alone
		{"open", false, false, "open"},
	} {
		if got := reconciledStatus(c.status, c.activePR, c.assigne); got != c.want {
			t.Errorf("reconciledStatus(%q, pr=%v, assigned=%v) = %q, want %q",
				c.status, c.activePR, c.assigne, got, c.want)
		}
	}
}
