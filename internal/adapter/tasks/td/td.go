// package: td
// type:    adapter (external tool)
// job:     the td integration the hub needs. Reads (list/get) go straight to
// td's SQLite for speed (sqlite.go); writes (set-status, close) go
// through the `td` CLI so td's invariants hold (D15). Both encapsulated
// here so callers see one adapter.
// limits:  knows nothing of openspec or rendering.
package td

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/flo-at/sindri/internal/hub/task"
)

// Tasks returns tasks matching the filter, ordered open → active → closed.
// Reads directly from td's SQLite db.
func Tasks(root string, f task.Filter) ([]task.Task, error) {
	return tasksFromDB(root, f)
}

// Source adapts td as a task source (its native td-* tasks). Always enabled — a
// missing td store surfaces as a Tasks error, matching the hub's expectation that
// td is the primary backend.
type Source struct{}

// Enabled reports whether this repo keeps a td store, so td gates on being in use like
// every other source. A repo that tracks work only in openspec or GitHub issues then lists
// those: its lack of a td store leaves td with nothing to contribute, rather than failing
// the sync for every source at once.
func (Source) Enabled(root string) bool { return HasStore(root) }

// Tasks returns all live td tasks as domain tasks (local read — force is moot).
func (Source) Tasks(root string, _ bool) ([]task.Task, error) { return Tasks(root, task.FilterAll) }

// OnMerged closes a merged td task via the self-close exception (its "done" close).
// A non-td id is ignored — this source only acts on td-* tasks.
func (Source) OnMerged(root, taskID, note string) error {
	if !strings.HasPrefix(taskID, "td-") {
		return nil
	}
	return Close(root, taskID, note)
}

// Finish closes (done) or deletes (scrap) a td task from the task list. handled is
// false for a non-td id, so the caller keeps asking the other sources.
func (Source) Finish(root, taskID string, scrap bool) (bool, error) {
	if !strings.HasPrefix(taskID, "td-") {
		return false, nil
	}
	if scrap {
		return true, Delete(root, taskID)
	}
	return true, Close(root, taskID, "closed from task list")
}

// Get loads a single task by ID (direct read).
func Get(root, id string) (task.Task, error) {
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

// Close closes a task via the self-close exception (used after a PR merge). td declines while an
// issue sits in review and points at `td approve`, which sindri can never use: it created and
// started the task, so td counts it as involved and refuses to let it review its own work. The way
// through is to leave review and close again — retried only when td still reports in_review, so any
// other refusal is reported as it stands.
func Close(root, id, reason string) error {
	err := mutate(root, "close", id, "--self-close-exception", reason)
	if err == nil {
		return nil
	}
	if t, gerr := Get(root, id); gerr != nil || t.Status != "in_review" {
		return err
	}
	if serr := SetStatus(root, id, "in_progress"); serr != nil {
		return fmt.Errorf("%w (and leaving review to retry failed: %v)", err, serr)
	}
	return mutate(root, "close", id, "--self-close-exception", reason)
}

// Delete soft-deletes a task (scrap) — a write, through the td tool. td's delete is
// restorable (`td restore`), so this discards without destroying.
func Delete(root, id string) error {
	return mutate(root, "delete", id)
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
	// td declines a mutation it considers out of policy and still exits 0, so the output decides.
	// A merge trusted that exit code, believed a refused close, and reopened finished work.
	if msg := refusal(s); msg != "" {
		return s, fmt.Errorf("td %s refused: %s", args[0], msg)
	}
	return s, nil
}

// refusal is the message from an ERROR line td printed while exiting 0, or "" when it printed none.
// Matched as a substring: the output is colourised, so the marker never starts the line.
func refusal(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if _, after, found := strings.Cut(l, "ERROR:"); found {
			return strings.TrimSpace(after)
		}
	}
	return ""
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
