// package: hub/store / workflow
// type:    persistence (SQLite, hub-owned)
// job:     the durable workflow state — the cached task read model (D15), each
//          agent's live workflow state (current task/branch/phase), and
//          merge-intents (PRs: branch + wants-merge + verdict). All write-through
//          so a crash loses nothing committed (D11).
// limits:  primitive columns only; mapping to/from issue.Task lives in the hub.
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const workflowSchema = `
CREATE TABLE IF NOT EXISTS tasks (
  id         TEXT PRIMARY KEY,
  title      TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT '',
  priority   TEXT NOT NULL DEFAULT '',
  type       TEXT NOT NULL DEFAULT '',
  labels     TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  synced_at  TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS agent_state (
  agent  TEXT PRIMARY KEY,
  task   TEXT NOT NULL DEFAULT '',
  branch TEXT NOT NULL DEFAULT '',
  phase  TEXT NOT NULL DEFAULT 'idle'  -- idle | working | submitted
);
CREATE TABLE IF NOT EXISTS prs (
  id         TEXT PRIMARY KEY,  -- pr-<task>
  task       TEXT NOT NULL DEFAULT '',
  agent      TEXT NOT NULL DEFAULT '',
  branch     TEXT NOT NULL DEFAULT '',
  base       TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT 'open', -- open | approved | rejected | merged
  feedback   TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT ''
);
`

// Task is the cached read-model row for a td task.
type Task struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Type     string `json:"type"`
	Labels   string `json:"labels"` // comma-joined
}

// AgentState is an agent's live workflow state (durable, D11).
type AgentState struct {
	Agent  string `json:"agent"`
	Task   string `json:"task"`
	Branch string `json:"branch"`
	Phase  string `json:"phase"`
}

// PR is a merge-intent: a branch its owner would like merged, plus a verdict.
type PR struct {
	ID        string `json:"id"`
	Task      string `json:"task"`
	Agent     string `json:"agent"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Status    string `json:"status"`
	Feedback  string `json:"feedback"`
	CreatedAt string `json:"created_at"`
}

// ReplaceTasks refreshes the cached task set in one transaction (the sync write
// path). Tasks absent from the new set are dropped so the cache mirrors td.
func (s *Store) ReplaceTasks(tasks []Task) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM tasks`); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range tasks {
		if _, err := tx.Exec(
			`INSERT INTO tasks (id,title,status,priority,type,labels,synced_at) VALUES (?,?,?,?,?,?,?)`,
			t.ID, t.Title, t.Status, t.Priority, t.Type, t.Labels, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertTask refreshes a single cached task (the point-of-use refresh path, D15).
func (s *Store) UpsertTask(t Task) error {
	_, err := s.db.Exec(`
		INSERT INTO tasks (id,title,status,priority,type,labels,synced_at)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, status=excluded.status, priority=excluded.priority,
			type=excluded.type, labels=excluded.labels, synced_at=excluded.synced_at`,
		t.ID, t.Title, t.Status, t.Priority, t.Type, t.Labels,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

// OpenTasks returns cached tasks with status "open", highest priority first.
func (s *Store) OpenTasks() ([]Task, error) {
	rows, err := s.db.Query(`
		SELECT id,title,status,priority,type,labels FROM tasks
		WHERE status='open'
		ORDER BY CASE priority
			WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2
			WHEN 'low' THEN 3 ELSE 4 END, id`)
	if err != nil {
		return nil, fmt.Errorf("open tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

func scanTasks(rows *sql.Rows) ([]Task, error) {
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.Priority, &t.Type, &t.Labels); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetState returns an agent's workflow state (zero value if none recorded).
func (s *Store) GetState(agent string) (AgentState, error) {
	st := AgentState{Agent: agent, Phase: "idle"}
	row := s.db.QueryRow(`SELECT task,branch,phase FROM agent_state WHERE agent=?`, agent)
	err := row.Scan(&st.Task, &st.Branch, &st.Phase)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("get state %s: %w", agent, err)
	}
	return st, nil
}

// SetState writes an agent's workflow state.
func (s *Store) SetState(st AgentState) error {
	if st.Phase == "" {
		st.Phase = "idle"
	}
	_, err := s.db.Exec(`
		INSERT INTO agent_state (agent,task,branch,phase) VALUES (?,?,?,?)
		ON CONFLICT(agent) DO UPDATE SET task=excluded.task, branch=excluded.branch, phase=excluded.phase`,
		st.Agent, st.Task, st.Branch, st.Phase)
	if err != nil {
		return fmt.Errorf("set state %s: %w", st.Agent, err)
	}
	return nil
}

// PutPR inserts or updates a merge-intent.
func (s *Store) PutPR(p PR) error {
	if p.CreatedAt == "" {
		p.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if p.Status == "" {
		p.Status = "open"
	}
	_, err := s.db.Exec(`
		INSERT INTO prs (id,task,agent,branch,base,status,feedback,created_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			task=excluded.task, agent=excluded.agent, branch=excluded.branch,
			base=excluded.base, status=excluded.status, feedback=excluded.feedback`,
		p.ID, p.Task, p.Agent, p.Branch, p.Base, p.Status, p.Feedback, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("put pr %s: %w", p.ID, err)
	}
	return nil
}

// GetPR returns a merge-intent by id.
func (s *Store) GetPR(id string) (PR, bool, error) {
	return s.scanPR(s.db.QueryRow(prCols+` WHERE id=?`, id))
}

// PRs returns merge-intents in the given statuses (empty = all), newest first.
func (s *Store) PRs(statuses ...string) ([]PR, error) {
	q := prCols
	var args []any
	if len(statuses) > 0 {
		q += ` WHERE status IN (` + placeholders(len(statuses)) + `)`
		for _, st := range statuses {
			args = append(args, st)
		}
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("prs: %w", err)
	}
	defer rows.Close()
	var out []PR
	for rows.Next() {
		p, err := scanPRRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

const prCols = `SELECT id,task,agent,branch,base,status,feedback,created_at FROM prs`

type scanner interface{ Scan(...any) error }

func (s *Store) scanPR(row scanner) (PR, bool, error) {
	p, err := scanPRRow(row)
	if err == sql.ErrNoRows {
		return PR{}, false, nil
	}
	if err != nil {
		return PR{}, false, err
	}
	return p, true, nil
}

func scanPRRow(row scanner) (PR, error) {
	var p PR
	err := row.Scan(&p.ID, &p.Task, &p.Agent, &p.Branch, &p.Base, &p.Status, &p.Feedback, &p.CreatedAt)
	return p, err
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
