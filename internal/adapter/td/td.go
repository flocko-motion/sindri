// package: td
// type:    adapter (external tool)
// job:     wraps the td task CLI — every td invocation (fetch + mutate) lives
//          here, converting td JSON into issue.Task.
// limits:  doesn't assemble issues (-> board) nor render them (-> render);
//          knows nothing of openspec (-> adapter/spec).
package td

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/issue"
)

// Tasks returns every task (including closed), ordered open → active → closed.
func Tasks(root string) ([]issue.Task, error) {
	out, err := run(root, "list", "--json", "--limit", "100", "--all")
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

// Show returns the human-readable detail of a task.
func Show(root, id string) (string, error) { return run(root, "show", id) }

// Comments returns the human-readable comments of a task.
func Comments(root, id string) (string, error) { return run(root, "comments", id) }

// Comment adds a comment to a task.
func Comment(root, id, body string) error { return mutate(root, "comment", id, body) }

// Reject moves a task from in_review back to open.
func Reject(root, id string) error { return mutate(root, "reject", id) }

// Close closes a task via the self-close exception (used after a PR merge).
func Close(root, id, reason string) error {
	return mutate(root, "close", id, "--self-close-exception", reason)
}

// SetStatus updates a task's status.
func SetStatus(root, id, status string) error {
	return mutate(root, "update", id, "--status", status)
}

// SetLabels replaces a task's labels.
func SetLabels(root, id string, labels []string) error {
	return mutate(root, "update", id, "--labels", strings.Join(labels, ","))
}

// CreateOpts are optional fields for Create.
type CreateOpts struct {
	Type     string
	Priority string
	Body     string
	Labels   []string
}

// Create creates a task and returns the td output (the new ID line).
func Create(root, title string, o CreateOpts) (string, error) {
	args := []string{"create", title}
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
	return run(root, args...)
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

// enrichConcurrency caps the parallel `td show` calls Enrich runs at once.
// 8 keeps wall time low without thrashing the td database.
const enrichConcurrency = 8

// Enrich populates fields td list strips out (currently: ParentID) by calling
// `td show <id> --json` per task. Calls run on a small worker pool so a
// 50-task board enriches in roughly one show-latency (~30ms) instead of the
// sequential ~300ms it would otherwise take. Per-task failures log to stderr
// and skip — partial enrichment beats a hard refresh failure, and the warning
// keeps the problem visible per "never fail silently".
func Enrich(root string, tasks []issue.Task) {
	if len(tasks) == 0 {
		return
	}
	sem := make(chan struct{}, enrichConcurrency)
	var wg sync.WaitGroup
	for i := range tasks {
		i := i
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			out, err := run(root, "show", tasks[i].ID, "--json")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: td enrich %s: %v\n", tasks[i].ID, err)
				return
			}
			var r rawTask
			if err := json.Unmarshal([]byte(out), &r); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: td enrich %s: parse: %v\n", tasks[i].ID, err)
				return
			}
			tasks[i].ParentID = r.ParentID
		}()
	}
	wg.Wait()
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
