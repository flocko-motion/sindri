// package: hub/store / owned
// type:    persistence (SQLite, hub-owned)
// job:     read and write the tasks sindri owns outright — the authoritative rows the
// task sync reads as one of its sources, and the only task table a mutation
// writes to.
// limits:  primitive columns and CRUD; ids, status vocabulary and the sync are the
// hub's (-> hub/workflow).
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OwnedTask is one task sindri owns. Status carries the lifecycle the workflow drives; the mirror
// in `tasks` is rebuilt from these rows and never written back. Parentage is deliberately absent:
// it belongs to every task, not only these, so it lives in task_parent (-> SetParent).
type OwnedTask struct {
	ID          string
	Title       string
	Status      string
	Priority    string
	Type        string
	Labels      string
	Description string
	CreatedAt   string
	UpdatedAt   string
}

const ownedCols = `id,title,status,priority,type,labels,description,created_at,updated_at`

// PutOwnedTask writes a task sindri owns, stamping updated_at and preserving created_at.
func (p *ProjectStore) PutOwnedTask(t OwnedTask) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	_, err := p.s.db.Exec(`
		INSERT INTO owned_tasks (project,`+ownedCols+`)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project,id) DO UPDATE SET
			title=excluded.title, status=excluded.status, priority=excluded.priority,
			type=excluded.type, labels=excluded.labels,
			description=excluded.description, updated_at=excluded.updated_at`,
		p.project, t.ID, t.Title, t.Status, t.Priority, t.Type, t.Labels, t.Description,
		t.CreatedAt, now)
	if err != nil {
		return fmt.Errorf("put owned task %s: %w", t.ID, err)
	}
	return nil
}

// OwnedTask reads one task; ok is false when this project owns no such id.
func (p *ProjectStore) OwnedTask(id string) (t OwnedTask, ok bool, err error) {
	row := p.s.db.QueryRow(`SELECT `+ownedCols+` FROM owned_tasks WHERE project=? AND id=?`, p.project, id)
	t, err = scanOwned(row)
	if errors.Is(err, sql.ErrNoRows) {
		return OwnedTask{}, false, nil
	}
	if err != nil {
		return OwnedTask{}, false, fmt.Errorf("owned task %s: %w", id, err)
	}
	return t, true, nil
}

// OwnedTasks reads every task this project owns, newest id order left to the caller.
func (p *ProjectStore) OwnedTasks() ([]OwnedTask, error) {
	rows, err := p.s.db.Query(`SELECT `+ownedCols+` FROM owned_tasks WHERE project=? ORDER BY id`, p.project)
	if err != nil {
		return nil, fmt.Errorf("owned tasks: %w", err)
	}
	defer rows.Close()
	var out []OwnedTask
	for rows.Next() {
		t, serr := scanOwned(rows)
		if serr != nil {
			return nil, fmt.Errorf("owned tasks: %w", serr)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetOwnedStatus moves a task through its lifecycle, the one field the workflow drives constantly.
func (p *ProjectStore) SetOwnedStatus(id, status string) error {
	return p.updateOwned(id, "status", status)
}

// SetOwnedPriority sets the priority that decides whether a worker may be handed the task.
func (p *ProjectStore) SetOwnedPriority(id, priority string) error {
	return p.updateOwned(id, "priority", priority)
}

// updateOwned writes one column, reporting an id this project does not own rather than passing
// silently: a status write that hit nothing is how a task drifts from what the board shows.
func (p *ProjectStore) updateOwned(id, column, value string) error {
	res, err := p.s.db.Exec(
		`UPDATE owned_tasks SET `+column+`=?, updated_at=? WHERE project=? AND id=?`,
		value, time.Now().UTC().Format(time.RFC3339), p.project, id)
	if err != nil {
		return fmt.Errorf("set %s on %s: %w", column, id, err)
	}
	if n, aerr := res.RowsAffected(); aerr == nil && n == 0 {
		return fmt.Errorf("set %s on %s: this project owns no such task", column, id)
	}
	return nil
}

// DeleteOwnedTask discards a task outright (the scrap path).
func (p *ProjectStore) DeleteOwnedTask(id string) error {
	if _, err := p.s.db.Exec(`DELETE FROM owned_tasks WHERE project=? AND id=?`, p.project, id); err != nil {
		return fmt.Errorf("delete owned task %s: %w", id, err)
	}
	return nil
}

// OwnsTask reports whether this project owns the id, so a caller can route a mutation.
func (p *ProjectStore) OwnsTask(id string) bool {
	var one int
	err := p.s.db.QueryRow(`SELECT 1 FROM owned_tasks WHERE project=? AND id=?`, p.project, id).Scan(&one)
	return err == nil
}

func scanOwned(r rowScanner) (OwnedTask, error) {
	var t OwnedTask
	err := r.Scan(&t.ID, &t.Title, &t.Status, &t.Priority, &t.Type, &t.Labels,
		&t.Description, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

// rowScanner is what QueryRow and Rows share, so one scan serves both.
type rowScanner interface{ Scan(...any) error }

// LabelList splits a stored label column; the column is comma-joined for the same reason the
// mirror's is, so the two agree without a translation step.
func LabelList(labels string) []string {
	var out []string
	for _, l := range strings.Split(labels, ",") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
