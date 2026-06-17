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

// CreateOpts are optional fields for Create.
type CreateOpts struct {
	Type     string
	Priority string
	Body     string
	Labels   []string
}

// Create creates a task and returns td's output (the new id line) — a write,
// through the td tool. The title is terminated with -- so a leading "--" in the
// title doesn't trip the flag parser.
func Create(root, title string, o CreateOpts) (string, error) {
	args := []string{"create"}
	if o.Type != "" {
		args = append(args, "-t", o.Type)
	}
	if o.Priority != "" {
		args = append(args, "-p", o.Priority)
	}
	if o.Body != "" {
		args = append(args, "-d", o.Body)
	}
	if len(o.Labels) > 0 {
		args = append(args, "--labels", strings.Join(o.Labels, ","))
	}
	args = append(args, "--", title)
	return run(root, args...)
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
