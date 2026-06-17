// package: td / sqlite
// type:    adapter (external tool — direct read path)
// job:     read tasks straight from td's SQLite db (.todos/issues.db) for speed,
//          bypassing the `td` CLI on the hot read path (D15). Writes still go
//          through the CLI (td.go); this is the encapsulated read-fast exception.
// limits:  read-only; couples to td's `issues` schema (id/title/status/type/
//          priority/labels/parent_id/timestamps, soft-deleted via deleted_at).
package td

import (
	"database/sql"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/issue"
	_ "modernc.org/sqlite"
)

// DBPath is td's SQLite database for a project.
func DBPath(root string) string { return filepath.Join(root, ".todos", "issues.db") }

const dbCols = `id, title, status, type, priority, labels, parent_id, created_at, updated_at`

// tasksFromDB reads all live tasks from td's db, applies the filter, and orders
// them open → active → closed (matching the CLI path).
func tasksFromDB(root string, f issue.Filter) ([]issue.Task, error) {
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
	var tasks []issue.Task
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

// Detail reads a task's long-form fields (description, acceptance) — not carried
// in issue.Task, fetched on demand for a detail view.
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

// taskFromDB reads a single task by id (live or not — Get is used post-mutation).
func taskFromDB(root, id string) (issue.Task, error) {
	db, err := sql.Open("sqlite", "file:"+DBPath(root))
	if err != nil {
		return issue.Task{}, err
	}
	defer db.Close()
	row := db.QueryRow(`SELECT `+dbCols+` FROM issues WHERE id=?`, id)
	return scanDBTask(row)
}

type rowScanner interface{ Scan(...any) error }

func scanDBTask(r rowScanner) (issue.Task, error) {
	var id, title, status, typ, priority, labels, parent, created, updated string
	if err := r.Scan(&id, &title, &status, &typ, &priority, &labels, &parent, &created, &updated); err != nil {
		return issue.Task{}, err
	}
	return issue.Task{
		ID: id, Title: title, Status: status, Type: typ, Priority: priority,
		ParentID: parent, Labels: splitLabels(labels),
		CreatedAt: parseTS(created), UpdatedAt: parseTS(updated),
	}, nil
}

// keep applies a Filter to a task (mirrors issue.Apply / the CLI's --all rule).
func keep(t issue.Task, f issue.Filter) bool {
	switch f {
	case issue.FilterClosed:
		return t.IsClosed()
	case issue.FilterOpen:
		return !t.IsClosed()
	default: // FilterAll
		return true
	}
}

// orderTasks sorts open → active → closed (active/closed by most-recently updated).
func orderTasks(items []issue.Task) []issue.Task {
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
	byUpdatedDesc := func(s []issue.Task) {
		sort.Slice(s, func(i, j int) bool { return s[i].UpdatedAt.After(s[j].UpdatedAt) })
	}
	byUpdatedDesc(active)
	byUpdatedDesc(closed)
	out := make([]issue.Task, 0, len(items))
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
