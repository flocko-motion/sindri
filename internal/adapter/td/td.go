// package: td
// type:    adapter (external tool)
// job:     the td integration the hub needs. Reads (list/get) go straight to
//          td's SQLite for speed (sqlite.go); writes (set-status, close) go
//          through the `td` CLI so td's invariants hold (D15). Both encapsulated
//          here so callers see one adapter.
// limits:  knows nothing of openspec or rendering.
package td

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/flo-at/sindri/internal/issue"
)

// Tasks returns tasks matching the filter, ordered open → active → closed.
// Reads directly from td's SQLite db.
func Tasks(root string, f issue.Filter) ([]issue.Task, error) {
	return tasksFromDB(root, f)
}

// Get loads a single task by ID (direct read).
func Get(root, id string) (issue.Task, error) {
	return taskFromDB(root, id)
}

// SetStatus updates a task's status — a write, so through the td tool.
func SetStatus(root, id, status string) error {
	return mutate(root, "update", id, "--status", status)
}

// Close closes a task via the self-close exception (used after a PR merge) — a
// write, through the td tool.
func Close(root, id, reason string) error {
	return mutate(root, "close", id, "--self-close-exception", reason)
}

// run executes td -w <root> <args...> and returns trimmed combined output.
func run(root string, args ...string) (string, error) {
	full := append([]string{"-w", root}, args...)
	out, err := exec.Command("td", full...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("td %s: %s", strings.Join(args, " "), s)
	}
	return s, nil
}

func mutate(root string, args ...string) error {
	_, err := run(root, args...)
	return err
}
