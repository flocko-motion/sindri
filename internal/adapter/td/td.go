// package: td
// type:    adapter (external tool)
// job:     wraps the td task CLI — the reads and writes the hub needs (list,
//          show, set-status, close), converting td JSON into issue.Task. Reads
//          go through the CLI today; writes always go through the tool (D15).
// limits:  doesn't assemble or render tasks; knows nothing of openspec.
package td

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/issue"
)

// maxTasks is a generous cap so a busy board is never silently truncated. td
// reads a local SQLite db, so a high limit is cheap.
const maxTasks = "100000"

// Tasks returns tasks matching the given filter, ordered open → active → closed.
func Tasks(root string, f issue.Filter) ([]issue.Task, error) {
	args := []string{"list", "--json", "--limit", maxTasks}
	if f != issue.FilterOpen {
		args = append(args, "--all")
	}
	out, err := run(root, args...)
	if err != nil {
		return nil, err
	}
	return parseAndSort([]byte(out))
}

// Get loads a single task by ID.
func Get(root, id string) (issue.Task, error) {
	out, err := run(root, "show", id, "--json")
	if err != nil {
		return issue.Task{}, err
	}
	var r rawTask
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return issue.Task{}, err
	}
	return r.toTask(), nil
}

// SetStatus updates a task's status.
func SetStatus(root, id, status string) error {
	return mutate(root, "update", id, "--status", status)
}

// Close closes a task via the self-close exception (used after a PR merge).
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

type rawTask struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Type      string   `json:"type"`
	Priority  string   `json:"priority"`
	ParentID  string   `json:"parent_id"`
	Labels    []string `json:"labels"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

func (r rawTask) toTask() issue.Task {
	created, _ := time.Parse(time.RFC3339Nano, r.CreatedAt)
	updated, _ := time.Parse(time.RFC3339Nano, r.UpdatedAt)
	return issue.Task{
		ID: r.ID, Title: r.Title, Status: r.Status, Type: r.Type,
		Priority: r.Priority, ParentID: r.ParentID,
		Labels: r.Labels, CreatedAt: created, UpdatedAt: updated,
	}
}

// parseAndSort parses td list JSON and orders it open → active → closed.
func parseAndSort(jsonData []byte) ([]issue.Task, error) {
	var raw []rawTask
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		return nil, err
	}
	items := make([]issue.Task, len(raw))
	for i, r := range raw {
		items[i] = r.toTask()
	}
	var open, active, closed []issue.Task
	for _, t := range items {
		switch {
		case t.IsActive():
			active = append(active, t)
		case t.IsClosed():
			closed = append(closed, t)
		default:
			open = append(open, t)
		}
	}
	byUpdatedDesc := func(s []issue.Task) func(i, j int) bool {
		return func(i, j int) bool { return s[i].UpdatedAt.After(s[j].UpdatedAt) }
	}
	sort.Slice(active, byUpdatedDesc(active))
	sort.Slice(closed, byUpdatedDesc(closed))
	result := make([]issue.Task, 0, len(items))
	result = append(result, open...)
	result = append(result, active...)
	result = append(result, closed...)
	return result, nil
}
