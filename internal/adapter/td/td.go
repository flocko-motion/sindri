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
	Parent   string // when set, the new task is created as a child of this id
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
	if o.Parent != "" {
		args = append(args, "--parent", o.Parent)
	}
	args = append(args, "--", title)
	return run(root, args...)
}

// SetStatus updates a task's status — a write, so through the td tool.
func SetStatus(root, id, status string) error {
	return mutate(root, "update", id, "--status", status)
}

// SetPriority updates a task's priority (P0…P4) — a write, through the td tool.
func SetPriority(root, id, priority string) error {
	return mutate(root, "update", id, "--priority", priority)
}

// UpdateOpts are the editable fields; zero values are left unchanged.
type UpdateOpts struct {
	Title    string
	Type     string
	Priority string
	Body     string // description
	Labels   []string
	Parent   string // re-parent under this id
}

// Update edits a task through the td tool, sending only the set fields.
func Update(root, id string, o UpdateOpts) error {
	args := []string{"update", id}
	if o.Type != "" {
		args = append(args, "--type", o.Type)
	}
	if o.Priority != "" {
		args = append(args, "--priority", o.Priority)
	}
	if o.Labels != nil {
		args = append(args, "--labels", strings.Join(o.Labels, ","))
	}
	if o.Body != "" {
		args = append(args, "-d", o.Body)
	}
	if o.Title != "" {
		args = append(args, "--title", o.Title)
	}
	if o.Parent != "" {
		args = append(args, "--parent", o.Parent)
	}
	return mutate(root, args...)
}

// Close closes a task via the self-close exception (used after a PR merge) — a
// write, through the td tool.
func Close(root, id, reason string) error {
	return mutate(root, "close", id, "--self-close-exception", reason)
}

// run executes td -w <root> <args...> and returns trimmed combined output. On
// failure it reports just td's error message, not its whole usage screen (cobra
// dumps the full --help to stderr on any error — too noisy to surface verbatim).
func run(root string, args ...string) (string, error) {
	full := append([]string{"-w", root}, args...)
	out, err := exec.Command("td", full...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("td %s: %s", args[0], tdErrorMessage(s))
	}
	return s, nil
}

// tdErrorMessage distills td's combined output down to its actual error: the
// line cobra prints as "Error: <msg>". Falls back to the last non-empty line
// (then the whole output) when there's no such line, so nothing is ever lost.
func tdErrorMessage(out string) string {
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		if msg, ok := strings.CutPrefix(strings.TrimSpace(l), "Error:"); ok {
			return strings.TrimSpace(msg)
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return out
}

func mutate(root string, args ...string) error {
	_, err := run(root, args...)
	return err
}
