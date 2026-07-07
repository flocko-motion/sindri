// package: hub/store / workflow
// type:    persistence (SQLite, hub-owned)
// job:     the durable workflow state — the cached task read model (D15), each
//          agent's live workflow state, and merge-intents (PRs) — all write-through
//          so a crash loses nothing committed (D11). Every table is project-keyed;
//          the methods hang off ProjectStore (scoped) except AllPRs (global board).
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
  project    TEXT NOT NULL,
  id         TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT '',
  priority   TEXT NOT NULL DEFAULT '',
  type       TEXT NOT NULL DEFAULT '',
  labels     TEXT NOT NULL DEFAULT '',
  parent_id  TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  synced_at  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, id)
);
CREATE TABLE IF NOT EXISTS agent_state (
  project   TEXT NOT NULL,
  agent     TEXT NOT NULL,
  task      TEXT NOT NULL DEFAULT '',
  branch    TEXT NOT NULL DEFAULT '',
  phase     TEXT NOT NULL DEFAULT 'idle',  -- idle | working | submitted
  container TEXT NOT NULL DEFAULT '',      -- container task held in the collaborative workflow ('' = structured)
  PRIMARY KEY (project, agent)
);
CREATE TABLE IF NOT EXISTS prs (
  project    TEXT NOT NULL,
  id         TEXT NOT NULL,  -- pr-<task>
  task       TEXT NOT NULL DEFAULT '',
  agent      TEXT NOT NULL DEFAULT '',
  branch     TEXT NOT NULL DEFAULT '',
  base       TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT 'open', -- open | approved | rejected | merged
  feedback   TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, id)
);
-- Durable priority we assign to tasks in our own db — survives the task-cache
-- rebuild. Used mainly for openspec items, which have no source priority.
CREATE TABLE IF NOT EXISTS task_priority (
  project  TEXT NOT NULL,
  id       TEXT NOT NULL,
  priority TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, id)
);
-- Review items attached to a PR. One row per requirement; its lifecycle is read
-- from which fields are filled: unassigned (created_at) → in progress (author +
-- review_at) → done (verdict + result + verdict_at).
CREATE TABLE IF NOT EXISTS reviews (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  project     TEXT NOT NULL,
  pr          TEXT NOT NULL,
  requirement TEXT NOT NULL DEFAULT '',
  author      TEXT NOT NULL DEFAULT '',  -- assigned reviewer ("" = unassigned)
  verdict     TEXT NOT NULL DEFAULT '',  -- pass | changes | fail ("" = not done)
  result      TEXT NOT NULL DEFAULT '',  -- the reviewer's findings
  created_at  TEXT NOT NULL DEFAULT '',  -- requirement added
  review_at   TEXT NOT NULL DEFAULT '',  -- picked up by an agent
  verdict_at  TEXT NOT NULL DEFAULT ''   -- verdict given
);
-- The latest lint result for a PR (so it persists across hub restarts).
CREATE TABLE IF NOT EXISTS pr_lint (
  project TEXT NOT NULL,
  pr      TEXT NOT NULL,
  output  TEXT NOT NULL DEFAULT '',
  ran_at  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, pr)
);
-- A PR's lifecycle history, shown in the detail column with timestamps.
CREATE TABLE IF NOT EXISTS pr_events (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  project TEXT NOT NULL,
  pr      TEXT NOT NULL,
  ts      TEXT NOT NULL,
  type    TEXT NOT NULL,
  payload TEXT NOT NULL DEFAULT ''
);
-- The hub-side approval gate for planner-created tasks.
CREATE TABLE IF NOT EXISTS task_approval (
  project TEXT NOT NULL,
  task    TEXT NOT NULL,
  status  TEXT NOT NULL DEFAULT 'pending', -- pending | approved | rejected
  comment TEXT NOT NULL DEFAULT '',
  at      TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, task)
);
`

// Task is the cached read-model row for a td task. Description/Acceptance are not
// cached (they can be large) — populated only on a detail read.
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
	// Approval is the hub-side gate on planner-created tasks: "" = none (a normal
	// task, claimable), pending (awaiting the user), approved (claimable), or
	// rejected (with ApprovalComment). Workers only ever see "" / approved tasks.
	Approval        string `json:"approval,omitempty"`
	ApprovalComment string `json:"approval_comment,omitempty"`
}

// AgentState is an agent's live workflow state (durable, D11).
type AgentState struct {
	Agent     string `json:"agent"`
	Task      string `json:"task"`
	Branch    string `json:"branch"`
	Phase     string `json:"phase"`
	Container string `json:"container,omitempty"`
}

// Review is one review item attached to a PR.
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

// PR is a merge-intent: a branch its owner would like merged, plus a verdict. It
// carries its project so the global board can tag which repo it belongs to.
type PR struct {
	Project   string `json:"project"`
	ID        string `json:"id"`
	Task      string `json:"task"`
	Agent     string `json:"agent"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Status    string `json:"status"`
	Feedback  string `json:"feedback"`
	CreatedAt string `json:"created_at"`
}

// ReplaceTasks refreshes this project's cached task set in one transaction. Tasks
// absent from the new set are dropped so the cache mirrors td.
func (p *ProjectStore) ReplaceTasks(tasks []Task) error {
	tx, err := p.s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM tasks WHERE project=?`, p.project); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range tasks {
		if _, err := tx.Exec(
			`INSERT INTO tasks (project,id,title,status,priority,type,labels,parent_id,synced_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			p.project, t.ID, t.Title, t.Status, t.Priority, t.Type, t.Labels, t.ParentID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertTask refreshes a single cached task in this project (point-of-use refresh).
func (p *ProjectStore) UpsertTask(t Task) error {
	_, err := p.s.db.Exec(`
		INSERT INTO tasks (project,id,title,status,priority,type,labels,parent_id,synced_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project,id) DO UPDATE SET
			title=excluded.title, status=excluded.status, priority=excluded.priority,
			type=excluded.type, labels=excluded.labels, parent_id=excluded.parent_id,
			synced_at=excluded.synced_at`,
		p.project, t.ID, t.Title, t.Status, t.Priority, t.Type, t.Labels, t.ParentID,
		time.Now().UTC().Format(time.RFC3339))
	return err
}

// taskCols is the shared SELECT projection: the cached td fields plus the hub-side
// approval overlay (empty when there's no approval row). The join is project-matched.
const taskCols = `t.id,t.title,t.status,t.priority,t.type,t.labels,t.parent_id,
	COALESCE(a.status,''), COALESCE(a.comment,'')`

const taskFrom = ` FROM tasks t LEFT JOIN task_approval a ON a.task=t.id AND a.project=t.project`

// OpenTasks returns this project's claimable tasks: status "open" and not gated by
// an unresolved approval, highest priority first.
func (p *ProjectStore) OpenTasks() ([]Task, error) {
	rows, err := p.s.db.Query(`
		SELECT `+taskCols+taskFrom+`
		WHERE t.project=? AND t.status='open' AND (a.status IS NULL OR a.status='approved')
		ORDER BY CASE WHEN t.priority='' THEN 1 ELSE 0 END, t.priority, t.id`, p.project)
	if err != nil {
		return nil, fmt.Errorf("open tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// OpenLeaves returns the claimable tasks the automatic assigner may take in this
// project: open, approved leaves, excluding any child of a container an agent
// holds. A task with no priority is left out — no priority, no assignment: an
// unprioritized task stays in the backlog (visible, editable) until a human sets a
// priority, which is the signal that it's ready to be worked.
func (p *ProjectStore) OpenLeaves() ([]Task, error) {
	rows, err := p.s.db.Query(`
		SELECT `+taskCols+taskFrom+`
		WHERE t.project=? AND t.status='open' AND (a.status IS NULL OR a.status='approved')
		  AND t.priority != ''
		  AND t.id NOT IN (SELECT parent_id FROM tasks WHERE project=? AND parent_id != '')
		  AND t.parent_id NOT IN (SELECT container FROM agent_state WHERE project=? AND container != '')
		ORDER BY t.priority, t.id`,
		p.project, p.project, p.project)
	if err != nil {
		return nil, fmt.Errorf("open leaves: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// OpenChildren returns a container's open, approved children in this project.
func (p *ProjectStore) OpenChildren(parentID string) ([]Task, error) {
	rows, err := p.s.db.Query(`
		SELECT `+taskCols+taskFrom+`
		WHERE t.project=? AND t.status='open' AND (a.status IS NULL OR a.status='approved') AND t.parent_id=?
		ORDER BY CASE WHEN t.priority='' THEN 1 ELSE 0 END, t.priority, t.id`, p.project, parentID)
	if err != nil {
		return nil, fmt.Errorf("open children of %s: %w", parentID, err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// GetTask returns a single cached task in this project (with its approval overlay).
func (p *ProjectStore) GetTask(id string) (Task, bool, error) {
	row := p.s.db.QueryRow(`SELECT `+taskCols+taskFrom+` WHERE t.project=? AND t.id=?`, p.project, id)
	var t Task
	err := row.Scan(&t.ID, &t.Title, &t.Status, &t.Priority, &t.Type, &t.Labels, &t.ParentID, &t.Approval, &t.ApprovalComment)
	if err == sql.ErrNoRows {
		return Task{}, false, nil
	}
	if err != nil {
		return Task{}, false, fmt.Errorf("get task %s: %w", id, err)
	}
	return t, true, nil
}

// MarkedContainers returns this project's tasks eligible for collaborative
// assignment: not closed, carrying the mark label, with an open child, unheld.
func (p *ProjectStore) MarkedContainers(label string) ([]Task, error) {
	rows, err := p.s.db.Query(`
		SELECT `+taskCols+taskFrom+`
		WHERE t.project=? AND t.status NOT IN ('closed','approved','merged')
		  AND (a.status IS NULL OR a.status='approved')
		  AND (',' || t.labels || ',') LIKE '%,' || ? || ',%'
		  AND EXISTS (SELECT 1 FROM tasks c WHERE c.project=t.project AND c.parent_id=t.id AND c.status='open')
		  AND t.id NOT IN (SELECT container FROM agent_state WHERE project=? AND container != '')
		ORDER BY CASE WHEN t.priority='' THEN 1 ELSE 0 END, t.priority, t.id`, p.project, label, p.project)
	if err != nil {
		return nil, fmt.Errorf("marked containers: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// AllTasks returns every cached task in this project with its approval overlay.
func (p *ProjectStore) AllTasks() ([]Task, error) {
	rows, err := p.s.db.Query(`
		SELECT `+taskCols+taskFrom+`
		WHERE t.project=?
		ORDER BY CASE WHEN t.priority='' THEN 1 ELSE 0 END, t.priority, t.id`, p.project)
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
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.Priority, &t.Type, &t.Labels, &t.ParentID, &t.Approval, &t.ApprovalComment); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetPriorityOverride records a priority we assign in our own db for this project.
func (p *ProjectStore) SetPriorityOverride(id, priority string) error {
	_, err := p.s.db.Exec(
		`INSERT INTO task_priority (project,id,priority) VALUES (?,?,?)
		 ON CONFLICT(project,id) DO UPDATE SET priority=excluded.priority`, p.project, id, priority)
	if err != nil {
		return fmt.Errorf("set priority override %s: %w", id, err)
	}
	return nil
}

// SetApproval records a task's approval state and comment in this project, now.
func (p *ProjectStore) SetApproval(task, status, comment string) error {
	_, err := p.s.db.Exec(
		`INSERT INTO task_approval (project,task,status,comment,at) VALUES (?,?,?,?,?)
		 ON CONFLICT(project,task) DO UPDATE SET status=excluded.status, comment=excluded.comment, at=excluded.at`,
		p.project, task, status, comment, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("set approval %s: %w", task, err)
	}
	return nil
}

// GetApproval returns a task's approval status and comment in this project.
func (p *ProjectStore) GetApproval(task string) (status, comment string) {
	_ = p.s.db.QueryRow(`SELECT status, comment FROM task_approval WHERE project=? AND task=?`, p.project, task).Scan(&status, &comment)
	return status, comment
}

// PriorityOverrides returns id→priority for this project's locally-assigned priorities.
func (p *ProjectStore) PriorityOverrides() (map[string]string, error) {
	rows, err := p.s.db.Query(`SELECT id, priority FROM task_priority WHERE project=?`, p.project)
	if err != nil {
		return nil, fmt.Errorf("priority overrides: %w", err)
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var id, pr string
		if err := rows.Scan(&id, &pr); err != nil {
			return nil, err
		}
		m[id] = pr
	}
	return m, rows.Err()
}

// GetState returns an agent's workflow state in this project (zero value if none).
func (p *ProjectStore) GetState(agent string) (AgentState, error) {
	st := AgentState{Agent: agent, Phase: "idle"}
	row := p.s.db.QueryRow(`SELECT task,branch,phase,container FROM agent_state WHERE project=? AND agent=?`, p.project, agent)
	err := row.Scan(&st.Task, &st.Branch, &st.Phase, &st.Container)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("get state %s: %w", agent, err)
	}
	return st, nil
}

// SetState writes an agent's workflow state in this project.
func (p *ProjectStore) SetState(st AgentState) error {
	if st.Phase == "" {
		st.Phase = "idle"
	}
	_, err := p.s.db.Exec(`
		INSERT INTO agent_state (project,agent,task,branch,phase,container) VALUES (?,?,?,?,?,?)
		ON CONFLICT(project,agent) DO UPDATE SET task=excluded.task, branch=excluded.branch, phase=excluded.phase, container=excluded.container`,
		p.project, st.Agent, st.Task, st.Branch, st.Phase, st.Container)
	if err != nil {
		return fmt.Errorf("set state %s: %w", st.Agent, err)
	}
	return nil
}

// PutPR inserts or updates a merge-intent in this project.
func (p *ProjectStore) PutPR(pr PR) error {
	if pr.CreatedAt == "" {
		pr.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if pr.Status == "" {
		pr.Status = "open"
	}
	_, err := p.s.db.Exec(`
		INSERT INTO prs (project,id,task,agent,branch,base,status,feedback,created_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project,id) DO UPDATE SET
			task=excluded.task, agent=excluded.agent, branch=excluded.branch,
			base=excluded.base, status=excluded.status, feedback=excluded.feedback`,
		p.project, pr.ID, pr.Task, pr.Agent, pr.Branch, pr.Base, pr.Status, pr.Feedback, pr.CreatedAt)
	if err != nil {
		return fmt.Errorf("put pr %s: %w", pr.ID, err)
	}
	return nil
}

// GetPR returns a merge-intent by id in this project.
func (p *ProjectStore) GetPR(id string) (PR, bool, error) {
	return scanPR(p.s.db.QueryRow(prCols+` WHERE project=? AND id=?`, p.project, id))
}

// PRs returns this project's merge-intents in the given statuses (empty = all).
func (p *ProjectStore) PRs(statuses ...string) ([]PR, error) {
	q := prCols + ` WHERE project=?`
	args := []any{p.project}
	if len(statuses) > 0 {
		q += ` AND status IN (` + placeholders(len(statuses)) + `)`
		for _, st := range statuses {
			args = append(args, st)
		}
	}
	q += ` ORDER BY created_at DESC`
	return queryPRs(p.s.db, q, args...)
}

// AllPRs returns merge-intents across all projects in the given statuses (empty =
// all), newest first — the global board read.
func (s *Store) AllPRs(statuses ...string) ([]PR, error) {
	q := prCols
	var args []any
	if len(statuses) > 0 {
		q += ` WHERE status IN (` + placeholders(len(statuses)) + `)`
		for _, st := range statuses {
			args = append(args, st)
		}
	}
	q += ` ORDER BY created_at DESC`
	return queryPRs(s.db, q, args...)
}

const prCols = `SELECT project,id,task,agent,branch,base,status,feedback,created_at FROM prs`

type scanner interface{ Scan(...any) error }

func queryPRs(db *sql.DB, q string, args ...any) ([]PR, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("prs: %w", err)
	}
	defer rows.Close()
	var out []PR
	for rows.Next() {
		pr, err := scanPRRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

func scanPR(row scanner) (PR, bool, error) {
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
	err := row.Scan(&p.Project, &p.ID, &p.Task, &p.Agent, &p.Branch, &p.Base, &p.Status, &p.Feedback, &p.CreatedAt)
	return p, err
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// --- pr lint ---

// SetPRLint stores (or replaces) a PR's latest lint output in this project, now.
func (p *ProjectStore) SetPRLint(prID, output string) error {
	_, err := p.s.db.Exec(`INSERT INTO pr_lint (project, pr, output, ran_at) VALUES (?,?,?,?)
		ON CONFLICT(project, pr) DO UPDATE SET output=excluded.output, ran_at=excluded.ran_at`,
		p.project, prID, output, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("set pr lint %s: %w", prID, err)
	}
	return nil
}

// GetPRLint returns a PR's stored lint output and run time in this project.
func (p *ProjectStore) GetPRLint(prID string) (output, ranAt string) {
	_ = p.s.db.QueryRow(`SELECT output, ran_at FROM pr_lint WHERE project=? AND pr=?`, p.project, prID).Scan(&output, &ranAt)
	return output, ranAt
}

// --- pr history ---

// LogPR appends a lifecycle event to a PR's history in this project (now, UTC).
func (p *ProjectStore) LogPR(prID, typ, payload string) error {
	_, err := p.s.db.Exec(`INSERT INTO pr_events (project, pr, ts, type, payload) VALUES (?,?,?,?,?)`,
		p.project, prID, time.Now().UTC().Format(time.RFC3339), typ, payload)
	if err != nil {
		return fmt.Errorf("log pr event %s: %w", prID, err)
	}
	return nil
}

// PREvents returns a PR's history in this project, oldest-first. Event.Agent
// carries the PR id (the table is keyed by PR, not agent).
func (p *ProjectStore) PREvents(prID string) ([]Event, error) {
	rows, err := p.s.db.Query(`SELECT id, pr, ts, type, payload FROM pr_events WHERE project=? AND pr=? ORDER BY id`, p.project, prID)
	if err != nil {
		return nil, fmt.Errorf("pr events for %s: %w", prID, err)
	}
	defer rows.Close()
	var evs []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Agent, &e.TS, &e.Type, &e.Payload); err != nil {
			return nil, fmt.Errorf("scan pr event: %w", err)
		}
		evs = append(evs, e)
	}
	return evs, rows.Err()
}

// --- reviews ---

const reviewCols = `SELECT id,pr,requirement,author,verdict,result,created_at,review_at,verdict_at FROM reviews`

// AddReview attaches a requirement to a PR in this project, unassigned. Returns id.
func (p *ProjectStore) AddReview(pr, requirement string) (int64, error) {
	res, err := p.s.db.Exec(`INSERT INTO reviews (project, pr, requirement, created_at) VALUES (?,?,?,?)`,
		p.project, pr, requirement, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("add review: %w", err)
	}
	return res.LastInsertId()
}

// AssignReview marks a review as picked up by an author (in progress).
func (p *ProjectStore) AssignReview(id int64, author string) error {
	_, err := p.s.db.Exec(`UPDATE reviews SET author=?, review_at=? WHERE id=? AND project=?`,
		author, time.Now().UTC().Format(time.RFC3339), id, p.project)
	if err != nil {
		return fmt.Errorf("assign review %d: %w", id, err)
	}
	return nil
}

// RecordVerdict completes a review with a verdict and the reviewer's findings.
func (p *ProjectStore) RecordVerdict(id int64, verdict, result string) error {
	_, err := p.s.db.Exec(`UPDATE reviews SET verdict=?, result=?, verdict_at=? WHERE id=? AND project=?`,
		verdict, result, time.Now().UTC().Format(time.RFC3339), id, p.project)
	if err != nil {
		return fmt.Errorf("record verdict %d: %w", id, err)
	}
	return nil
}

// Reviews lists a PR's review items in this project, oldest first.
func (p *ProjectStore) Reviews(pr string) ([]Review, error) {
	rows, err := p.s.db.Query(reviewCols+` WHERE project=? AND pr=? ORDER BY id`, p.project, pr)
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
