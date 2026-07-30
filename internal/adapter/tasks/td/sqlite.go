// package: td / sqlite
// type:    adapter (external tool — direct read path)
// job:     read tasks straight from td's SQLite db (.todos/issues.db) for speed,
// bypassing the `td` CLI on the hot read path (D15). Writes still go
// through the CLI (td.go); this is the encapsulated read-fast exception.
// limits:  read-only; couples to td's `issues` schema (id/title/status/type/
// priority/labels/parent_id/timestamps, soft-deleted via deleted_at).
package td

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/hub/task"
	_ "modernc.org/sqlite"
)

// DBPath is td's SQLite database for a project.
func DBPath(root string) string { return filepath.Join(root, ".todos", "issues.db") }

// HasStore gates td as a task source: a repo without one contributes no td tasks.
func HasStore(root string) bool {
	_, err := os.Stat(DBPath(root))
	return err == nil
}

// dbCols is shared by both queries and scanDBTask so they agree about order. COALESCE
// keeps a NULL description from failing a whole task read.
const dbCols = `id, title, COALESCE(description,''), status, type, priority, labels, parent_id, created_at, updated_at`

// tasksFromDB reads live tasks, filters, and orders open → active → closed like the CLI.
func tasksFromDB(root string, f task.Filter) ([]task.Task, error) {
	if _, err := os.Stat(DBPath(root)); err != nil {
		// Reads come from the repo root. Absent it, td's writes may route to a .todos
		// found elsewhere and diverge — fail loud rather than report zero tasks.
		return nil, fmt.Errorf("no td store at %s — the hub expects td's database at the repo root where it was launched; run `td` there to create one", DBPath(root))
	}
	db, err := sql.Open("sqlite", "file:"+DBPath(root))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT ` + dbCols + ` FROM issues WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []task.Task
	for rows.Next() {
		t, err := scanDBTask(rows)
		if err != nil {
			return nil, err
		}
		if keep(t, f) {
			tasks = append(tasks, t)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orderTasks(tasks), nil
}

// Detail fetches long-form fields on demand; task.Task doesn't carry them.
func Detail(root, id string) (description, acceptance string, err error) {
	db, err := sql.Open("sqlite", "file:"+DBPath(root))
	if err != nil {
		return "", "", err
	}
	defer db.Close()
	row := db.QueryRow(`SELECT description, acceptance FROM issues WHERE id=?`, id)
	if err := row.Scan(&description, &acceptance); err != nil && err != sql.ErrNoRows {
		return "", "", err
	}
	return description, acceptance, nil
}

// Comment is one td comment; Author is td's session_id, not a human.
type Comment struct {
	ID        string
	Author    string
	Body      string
	CreatedAt time.Time
}

// Comments reads a task's thread. nil, not an error, when older td has no comments
// table or the task has none: an optional source's absence isn't a failure.
func Comments(root, id string) ([]Comment, error) {
	if _, err := os.Stat(DBPath(root)); err != nil {
		return nil, nil
	}
	db, err := sql.Open("sqlite", "file:"+DBPath(root))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id, session_id, text, created_at FROM comments WHERE issue_id=? ORDER BY created_at`, id)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil // td build without the comments feature
		}
		return nil, err
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var cid, session, text, created string
		if err := rows.Scan(&cid, &session, &text, &created); err != nil {
			return nil, err
		}
		out = append(out, Comment{ID: cid, Author: session, Body: text, CreatedAt: parseTS(created)})
	}
	return out, rows.Err()
}

// taskFromDB reads a single task by id (live or not — Get is used post-mutation).
func taskFromDB(root, id string) (task.Task, error) {
	db, err := sql.Open("sqlite", "file:"+DBPath(root))
	if err != nil {
		return task.Task{}, err
	}
	defer db.Close()
	row := db.QueryRow(`SELECT `+dbCols+` FROM issues WHERE id=?`, id)
	return scanDBTask(row)
}

type rowScanner interface{ Scan(...any) error }

func scanDBTask(r rowScanner) (task.Task, error) {
	var id, title, desc, status, typ, priority, labels, parent, created, updated string
	if err := r.Scan(&id, &title, &desc, &status, &typ, &priority, &labels, &parent, &created, &updated); err != nil {
		return task.Task{}, err
	}
	return task.Task{
		ID: id, Title: title, Description: desc, Status: status, Type: typ, Priority: priority,
		ParentID: parent, Labels: splitLabels(labels),
		CreatedAt: parseTS(created), UpdatedAt: parseTS(updated),
	}, nil
}

// keep applies a Filter to a task (mirrors task.Apply / the CLI's --all rule).
func keep(t task.Task, f task.Filter) bool {
	switch f {
	case task.FilterClosed:
		return t.IsClosed()
	case task.FilterOpen:
		return !t.IsClosed()
	default: // FilterAll
		return true
	}
}

// orderTasks sorts open → active → closed (active/closed by most-recently updated).
func orderTasks(items []task.Task) []task.Task {
	var open, active, closed []task.Task
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
	byUpdatedDesc := func(s []task.Task) {
		sort.Slice(s, func(i, j int) bool { return s[i].UpdatedAt.After(s[j].UpdatedAt) })
	}
	byUpdatedDesc(active)
	byUpdatedDesc(closed)
	out := make([]task.Task, 0, len(items))
	out = append(out, open...)
	out = append(out, active...)
	return append(out, closed...)
}

func splitLabels(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseTS(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}
