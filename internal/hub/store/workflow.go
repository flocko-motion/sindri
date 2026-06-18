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
  parent_id  TEXT NOT NULL DEFAULT '',
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
-- Durable priority we assign to tasks in our own db — survives the task-cache
-- rebuild. Used mainly for openspec items, which have no source priority.
CREATE TABLE IF NOT EXISTS task_priority (
  id       TEXT PRIMARY KEY,
  priority TEXT NOT NULL DEFAULT ''
);
-- Review items attached to a PR. One row per requirement; its lifecycle is read
-- from which fields are filled: unassigned (created_at) → in progress (author +
-- review_at) → done (verdict + result + verdict_at).
CREATE TABLE IF NOT EXISTS reviews (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  pr          TEXT NOT NULL,
  requirement TEXT NOT NULL DEFAULT '',
  author      TEXT NOT NULL DEFAULT '',  -- assigned reviewer ("" = unassigned)
  verdict     TEXT NOT NULL DEFAULT '',  -- pass | changes | fail ("" = not done)
  result      TEXT NOT NULL DEFAULT '',  -- the reviewer's findings
  created_at  TEXT NOT NULL DEFAULT '',  -- requirement added
  review_at   TEXT NOT NULL DEFAULT '',  -- picked up by an agent
  verdict_at  TEXT NOT NULL DEFAULT ''   -- verdict given
);
-- The latest lint result for a PR (so it persists across hub restarts and is
-- shown without re-running until L re-runs it).
CREATE TABLE IF NOT EXISTS pr_lint (
  pr     TEXT PRIMARY KEY,
  output TEXT NOT NULL DEFAULT '',
  ran_at TEXT NOT NULL DEFAULT ''
);
`

// Task is the cached read-model row for a td task. Description/Acceptance are
// not cached (they can be large) — they are populated only on a detail read
// (TaskDetail), empty in board/list rows.
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	Type        string `json:"type"`
	Labels      string `json:"labels"` // comma-joined
	ParentID    string `json:"parent_id"`
	Description string `json:"description,omitempty"`
	Acceptance  string `json:"acceptance,omitempty"`
}

// AgentState is an agent's live workflow state (durable, D11).
type AgentState struct {
	Agent  string `json:"agent"`
	Task   string `json:"task"`
	Branch string `json:"branch"`
	Phase  string `json:"phase"`
}

// Review is one review item attached to a PR. Its lifecycle is read from which
// fields are filled: unassigned (only created_at) → in progress (author +
// review_at) → done (verdict + result + verdict_at). Requirement is the free-text
// instruction handed to the reviewing agent.
type Review struct {
	ID          int64  `json:"id"`
	PR          string `json:"pr"`
	Requirement string `json:"requirement"`
	Author      string `json:"author"`
	Verdict     string `json:"verdict"`
	Result      string `json:"result"`
	CreatedAt   string `json:"created_at"`
	ReviewAt    string `json:"review_at"`
	VerdictAt   string `json:"verdict_at"`
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
			`INSERT INTO tasks (id,title,status,priority,type,labels,parent_id,synced_at) VALUES (?,?,?,?,?,?,?,?)`,
			t.ID, t.Title, t.Status, t.Priority, t.Type, t.Labels, t.ParentID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertTask refreshes a single cached task (the point-of-use refresh path, D15).
func (s *Store) UpsertTask(t Task) error {
	_, err := s.db.Exec(`
		INSERT INTO tasks (id,title,status,priority,type,labels,parent_id,synced_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, status=excluded.status, priority=excluded.priority,
			type=excluded.type, labels=excluded.labels, parent_id=excluded.parent_id,
			synced_at=excluded.synced_at`,
		t.ID, t.Title, t.Status, t.Priority, t.Type, t.Labels, t.ParentID,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

// OpenTasks returns cached tasks with status "open", highest priority first.
func (s *Store) OpenTasks() ([]Task, error) {
	// td priorities are P1 (highest) … P4 (lowest); lexical order matches, with
	// unset priorities sorted last.
	rows, err := s.db.Query(`
		SELECT id,title,status,priority,type,labels,parent_id FROM tasks
		WHERE status='open'
		ORDER BY CASE WHEN priority='' THEN 1 ELSE 0 END, priority, id`)
	if err != nil {
		return nil, fmt.Errorf("open tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// AllTasks returns every cached task, highest priority first (unset last).
func (s *Store) AllTasks() ([]Task, error) {
	rows, err := s.db.Query(`
		SELECT id,title,status,priority,type,labels,parent_id FROM tasks
		ORDER BY CASE WHEN priority='' THEN 1 ELSE 0 END, priority, id`)
	if err != nil {
		return nil, fmt.Errorf("all tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

func scanTasks(rows *sql.Rows) ([]Task, error) {
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.Priority, &t.Type, &t.Labels, &t.ParentID); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetPriorityOverride records a priority we assign in our own db.
func (s *Store) SetPriorityOverride(id, priority string) error {
	_, err := s.db.Exec(
		`INSERT INTO task_priority (id,priority) VALUES (?,?)
		 ON CONFLICT(id) DO UPDATE SET priority=excluded.priority`, id, priority)
	if err != nil {
		return fmt.Errorf("set priority override %s: %w", id, err)
	}
	return nil
}

// PriorityOverrides returns id→priority for all locally-assigned priorities.
func (s *Store) PriorityOverrides() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT id, priority FROM task_priority`)
	if err != nil {
		return nil, fmt.Errorf("priority overrides: %w", err)
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var id, p string
		if err := rows.Scan(&id, &p); err != nil {
			return nil, err
		}
		m[id] = p
	}
	return m, rows.Err()
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

// --- pr lint ---

// SetPRLint stores (or replaces) a PR's latest lint output, stamped now.
func (s *Store) SetPRLint(prID, output string) error {
	_, err := s.db.Exec(`INSERT INTO pr_lint (pr, output, ran_at) VALUES (?,?,?)
		ON CONFLICT(pr) DO UPDATE SET output=excluded.output, ran_at=excluded.ran_at`,
		prID, output, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("set pr lint %s: %w", prID, err)
	}
	return nil
}

// GetPRLint returns a PR's stored lint output and run time ("" if never run).
func (s *Store) GetPRLint(prID string) (output, ranAt string) {
	_ = s.db.QueryRow(`SELECT output, ran_at FROM pr_lint WHERE pr=?`, prID).Scan(&output, &ranAt)
	return output, ranAt
}

// --- reviews ---

const reviewCols = `SELECT id,pr,requirement,author,verdict,result,created_at,review_at,verdict_at FROM reviews`

// AddReview attaches a requirement (free-text review instruction) to a PR,
// unassigned. Returns the new row id.
func (s *Store) AddReview(pr, requirement string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO reviews (pr, requirement, created_at) VALUES (?,?,?)`,
		pr, requirement, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("add review: %w", err)
	}
	return res.LastInsertId()
}

// AssignReview marks a review as picked up by an author (in progress).
func (s *Store) AssignReview(id int64, author string) error {
	_, err := s.db.Exec(`UPDATE reviews SET author=?, review_at=? WHERE id=?`,
		author, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("assign review %d: %w", id, err)
	}
	return nil
}

// RecordVerdict completes a review with a verdict and the reviewer's findings.
func (s *Store) RecordVerdict(id int64, verdict, result string) error {
	_, err := s.db.Exec(`UPDATE reviews SET verdict=?, result=?, verdict_at=? WHERE id=?`,
		verdict, result, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("record verdict %d: %w", id, err)
	}
	return nil
}

// Reviews lists a PR's review items, oldest first.
func (s *Store) Reviews(pr string) ([]Review, error) {
	rows, err := s.db.Query(reviewCols+` WHERE pr=? ORDER BY id`, pr)
	if err != nil {
		return nil, fmt.Errorf("reviews %s: %w", pr, err)
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.PR, &r.Requirement, &r.Author, &r.Verdict,
			&r.Result, &r.CreatedAt, &r.ReviewAt, &r.VerdictAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
