// package: hub/store / runs
// type:    adapter (SQLite, hub-owned)
// job:     the run-queue row: schedule, read, and the two mutations a human can make
// on one (cancel, reprioritise). The schema lives in workflow.go alongside its
// siblings; execution and scheduling policy are the hub's, not this file's.
// limits:  primitive columns only; queue order is derived, not stored (-> workflow/run.go).
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/flo-at/sindri/internal/api"
)

// Run is a scheduled command in this project's run queue; it crosses the wire, so it
// is internal/api.Run under the name every existing caller here already uses.
type Run = api.Run

// PutRun inserts or updates a run in this project. updated_at is stamped here,
// unconditionally, on every call — mirroring PutPR, for the same reason: the active filter
// trusts this column, never the caller's own idea of when it last changed.
func (p *ProjectStore) PutRun(r Run) error {
	if r.CreatedAt == "" {
		r.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if r.Status == "" {
		r.Status = "queued"
	}
	r.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := p.s.db.Exec(`
		INSERT INTO runs (project,id,agent,command,status,priority,timeout,workspace,task,kind,message,created_at,started_at,finished_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project,id) DO UPDATE SET
			agent=excluded.agent, command=excluded.command, status=excluded.status,
			priority=excluded.priority, timeout=excluded.timeout, workspace=excluded.workspace,
			task=excluded.task, kind=excluded.kind, message=excluded.message, started_at=excluded.started_at,
			finished_at=excluded.finished_at, updated_at=excluded.updated_at`,
		p.project, r.ID, r.Agent, r.Command, r.Status, r.Priority, r.Timeout, r.Workspace, r.Task,
		r.Kind, r.Message, r.CreatedAt, r.StartedAt, r.FinishedAt, r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("put run %s: %w", r.ID, err)
	}
	return nil
}

// GetRun returns a run by id in this project.
func (p *ProjectStore) GetRun(id string) (Run, bool, error) {
	return scanRun(p.s.db.QueryRow(runCols+` WHERE project=? AND id=?`, p.project, id))
}

// Runs returns this project's runs in the given statuses (empty = all), oldest first — the
// order a queue reads naturally, before priority reorders the still-queued ones.
func (p *ProjectStore) Runs(statuses ...string) ([]Run, error) {
	q := runCols + ` WHERE project=?`
	args := []any{p.project}
	if len(statuses) > 0 {
		q += ` AND status IN (` + placeholders(len(statuses)) + `)`
		for _, st := range statuses {
			args = append(args, st)
		}
	}
	q += ` ORDER BY created_at`
	return queryRuns(p.s.db, q, args...)
}

// AllRuns returns runs across every project in the given statuses (empty = all) — the global
// board read, matching AllPRs.
func (s *Store) AllRuns(statuses ...string) ([]Run, error) {
	q := runCols
	var args []any
	if len(statuses) > 0 {
		q += ` WHERE status IN (` + placeholders(len(statuses)) + `)`
		for _, st := range statuses {
			args = append(args, st)
		}
	}
	q += ` ORDER BY created_at`
	return queryRuns(s.db, q, args...)
}

// SetRunStatus moves a run to status, stamping finished_at once it leaves queued/running — a run
// never returns to either from a terminal state, so finished_at, once set, stands.
func (p *ProjectStore) SetRunStatus(id, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var err error
	switch status {
	case "queued":
		_, err = p.s.db.Exec(`UPDATE runs SET status=?, updated_at=? WHERE project=? AND id=?`,
			status, now, p.project, id)
	case "running":
		_, err = p.s.db.Exec(`UPDATE runs SET status=?, started_at=?, updated_at=? WHERE project=? AND id=?`,
			status, now, now, p.project, id)
	default: // terminal: passed | failed | timed_out | cancelled
		_, err = p.s.db.Exec(`UPDATE runs SET status=?, finished_at=?, updated_at=? WHERE project=? AND id=?`,
			status, now, now, p.project, id)
	}
	if err != nil {
		return fmt.Errorf("set run status %s: %w", id, err)
	}
	return nil
}

// SetRunResult records a run's terminal outcome in one write: status, its full uncapped output
// (capping happens only when fetched, -> capRunOutput), and the command's exit code.
func (p *ProjectStore) SetRunResult(id, status, output string, exitCode int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := p.s.db.Exec(`UPDATE runs SET status=?, output=?, exit_code=?, finished_at=?, updated_at=? WHERE project=? AND id=?`,
		status, output, exitCode, now, now, p.project, id)
	if err != nil {
		return fmt.Errorf("set run result %s: %w", id, err)
	}
	return nil
}

// SetRunPriority reprioritises a queued run; changing one already running or finished has
// nothing left to reorder, but the write itself is harmless, so the caller decides whether to
// refuse it.
func (p *ProjectStore) SetRunPriority(id, priority string) error {
	_, err := p.s.db.Exec(`UPDATE runs SET priority=?, updated_at=? WHERE project=? AND id=?`,
		priority, time.Now().UTC().Format(time.RFC3339), p.project, id)
	if err != nil {
		return fmt.Errorf("set run priority %s: %w", id, err)
	}
	return nil
}

// RunOutput returns a run's stored console output in this project, "" if none — kept off Run
// itself (-> api.RunDetail), the way a PR's diff is, since the list must never carry it.
func (p *ProjectStore) RunOutput(id string) (string, error) {
	var out string
	err := p.s.db.QueryRow(`SELECT output FROM runs WHERE project=? AND id=?`, p.project, id).Scan(&out)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("run output %s: %w", id, err)
	}
	return out, nil
}

const runCols = `SELECT project,id,agent,command,status,priority,timeout,workspace,task,exit_code,kind,message,created_at,started_at,finished_at,updated_at FROM runs`

func queryRuns(db *sql.DB, q string, args ...any) ([]Run, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("runs: %w", err)
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		r, err := scanRunRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanRun(row scanner) (Run, bool, error) {
	r, err := scanRunRow(row)
	if err == sql.ErrNoRows {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	return r, true, nil
}

func scanRunRow(row scanner) (Run, error) {
	var r Run
	err := row.Scan(&r.Project, &r.ID, &r.Agent, &r.Command, &r.Status, &r.Priority, &r.Timeout,
		&r.Workspace, &r.Task, &r.ExitCode, &r.Kind, &r.Message, &r.CreatedAt, &r.StartedAt, &r.FinishedAt, &r.UpdatedAt)
	return r, err
}
