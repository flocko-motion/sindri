// package: hub/store / workflow
// type:    adapter (SQLite, hub-owned)
// job:     the workflow schema (D11), plus each agent's live state and merge-intents
// (PRs), write-through so a crash loses nothing committed. Project-keyed,
// except AllPRs (global board). The task read model itself is tasks.go's.
// limits:  primitive columns only; mapping to/from issue.Task lives in the hub.
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
)

const workflowSchema = `
CREATE TABLE IF NOT EXISTS tasks (
  project    TEXT NOT NULL,
  id         TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT '',
  priority   TEXT NOT NULL DEFAULT '',
  tier       TEXT NOT NULL DEFAULT '', -- junior|mid|senior, '' unrated (-> api.TierOrDefault)
  type       TEXT NOT NULL DEFAULT '',
  labels      TEXT NOT NULL DEFAULT '',
  parent_id   TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '', -- the body (GitHub issue body, td/spec description)
  url         TEXT NOT NULL DEFAULT '', -- an external permalink (e.g. a GitHub issue); '' if none
  updated_at  TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT '', -- when the task began at its source; '' when it has no answer
  synced_at   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, id)
);
CREATE TABLE IF NOT EXISTS agent_state (
  project   TEXT NOT NULL,
  agent     TEXT NOT NULL,
  task      TEXT NOT NULL DEFAULT '',
  branch    TEXT NOT NULL DEFAULT '',
  phase     TEXT NOT NULL DEFAULT 'idle',  -- idle | working | submitted
  container TEXT NOT NULL DEFAULT '',      -- container task held in the collaborative workflow ('' = structured)
  -- The question an agent stopped on, waiting for the user to decide it ('' = not escalated). Written
  -- only by SetEscalation/ClearEscalation, never by SetState (-> SetState).
  escalation TEXT NOT NULL DEFAULT '',
  -- Notes to the user this agent may still send on its current claim (-> GrantNotes). Written only by
  -- GrantNotes/SetNotesLeft, never by SetState, for the same reason the escalation is not.
  notes_left INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (project, agent)
);
CREATE TABLE IF NOT EXISTS prs (
  project    TEXT NOT NULL,
  id         TEXT NOT NULL,  -- pr-<task>
  task       TEXT NOT NULL DEFAULT '',
  agent      TEXT NOT NULL DEFAULT '',
  branch     TEXT NOT NULL DEFAULT '',
  base       TEXT NOT NULL DEFAULT '',
  status     TEXT NOT NULL DEFAULT 'open', -- open | approved | rejected | merged | scrapped
  feedback   TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  kind       TEXT NOT NULL DEFAULT 'final', -- final (task-done) | interim (mid-task contribution to the reference branch)
  updated_at TEXT NOT NULL DEFAULT '', -- stamped by every PutPR, for the active filter (-> api.PRFilterActive)
  PRIMARY KEY (project, id)
);
-- The tasks sindri owns, and the authority for them. The tasks table above is a read model the
-- sync rebuilds from every source including this one, so durable state belongs here. Ids keep the
-- td- prefix, which PR ids, branch names and agent state all embed.
CREATE TABLE IF NOT EXISTS owned_tasks (
  project     TEXT NOT NULL,
  id          TEXT NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'open', -- open | in_progress | in_review | closed
  priority    TEXT NOT NULL DEFAULT '',
  tier        TEXT NOT NULL DEFAULT '', -- junior|mid|senior, '' unrated (-> api.TierOrDefault)
  type        TEXT NOT NULL DEFAULT 'task',
  labels      TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, id)
);
-- Parentage for EVERY task, whatever owns its text. The hierarchy is sindri's own reading of how
-- work relates, so an openspec change or a GitHub issue can be a parent or a child even though
-- neither carries the notion upstream. One home, so no task has two answers.
CREATE TABLE IF NOT EXISTS task_parent (
  project   TEXT NOT NULL,
  id        TEXT NOT NULL,
  parent_id TEXT NOT NULL,
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
  verdict_at  TEXT NOT NULL DEFAULT '',  -- verdict given
  advisory    INTEGER NOT NULL DEFAULT 0 -- a planner's optional badge; never satisfies the merge gate alone
);
-- The latest lint result for a PR (so it persists across hub restarts).
CREATE TABLE IF NOT EXISTS pr_lint (
  project TEXT NOT NULL,
  pr      TEXT NOT NULL,
  output  TEXT NOT NULL DEFAULT '',
  ran_at  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, pr)
);
-- What the gate said about one COMMIT, so an unchanged commit is never gated twice. It carries the
-- verify command too: re-pointing that asks a different question of the same tree.
CREATE TABLE IF NOT EXISTS gate_result (
  project TEXT NOT NULL,
  sha     TEXT NOT NULL,
  passed  INTEGER NOT NULL DEFAULT 0,
  verify  TEXT NOT NULL DEFAULT '',
  output  TEXT NOT NULL DEFAULT '',
  ran_at  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, sha)
);
-- Who is waiting to be TOLD a run has landed, beyond whoever queued it: a second asker joins the
-- queued run rather than queueing another, and would otherwise wait on a message nobody sends.
CREATE TABLE IF NOT EXISTS run_waiters (
  project TEXT NOT NULL,
  run     TEXT NOT NULL,
  agent   TEXT NOT NULL,
  PRIMARY KEY (project, run, agent)
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
-- The run queue: one scheduled command per row, its console output, and how it went.
-- Methods live in runs.go; the schema stays here alongside its siblings.
CREATE TABLE IF NOT EXISTS runs (
  project     TEXT NOT NULL,
  id          TEXT NOT NULL,
  agent       TEXT NOT NULL DEFAULT '',
  command     TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'queued', -- queued|running|passed|failed|timed_out|cancelled
  priority    TEXT NOT NULL DEFAULT '',       -- P0..P4, same vocabulary as tasks; '' sorts last
  timeout     TEXT NOT NULL DEFAULT '',       -- agent-requested budget, e.g. '5m'; '' = the hub's hard cap
  workspace   TEXT NOT NULL DEFAULT '',       -- the agent's worktree path at schedule time
  task        TEXT NOT NULL DEFAULT '',       -- the agent's task at schedule time, for staleness at dequeue
  exit_code   INTEGER NOT NULL DEFAULT 0,
  kind        TEXT NOT NULL DEFAULT '',       -- '' = an ordinary run; otherwise which gate (-> workflow/gate.go)
  message     TEXT NOT NULL DEFAULT '',       -- the agent's submit/contribute text, replayed once a gate passes
  commit_sha  TEXT NOT NULL DEFAULT '',       -- the commit a gate run checks; its verdict is recorded against it
  output      TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT '',
  started_at  TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT '', -- stamped on every write, for the active filter
  PRIMARY KEY (project, id)
);
`

// AgentState is an agent's live workflow state (durable, D11).
type AgentState struct {
	Agent     string `json:"agent"`
	Task      string `json:"task"`
	Branch    string `json:"branch"`
	Phase     string `json:"phase"`
	Container string `json:"container,omitempty"`
	// NotesLeft is how many notes to the user this agent may still send on its current claim. Here
	// rather than derived, because the grant is per CLAIM and replaces (-> GrantNotes).
	NotesLeft int `json:"notesLeft,omitempty"`
	// Escalation is the question the agent stopped on, waiting for the user to decide it ('' = not
	// escalated). It rides here so every reader of the state has it — the command surface, the board,
	// the directive — but it is NOT part of what SetState writes (-> SetState).
	Escalation string `json:"escalation,omitempty"`
}

// Review is one review item attached to a PR; it crosses the wire, so it is
// internal/api.Review under the name every existing caller here already uses.
type Review = api.Review

// PR is a merge-intent; it crosses the wire, so it is internal/api.PR under the name
// every existing caller here already uses.
type PR = api.PR

// GetState returns an agent's workflow state in this project (zero value if none).
func (p *ProjectStore) GetState(agent string) (AgentState, error) {
	st := AgentState{Agent: agent, Phase: "idle"}
	row := p.s.db.QueryRow(`SELECT task,branch,phase,container,escalation,notes_left FROM agent_state WHERE project=? AND agent=?`, p.project, agent)
	err := row.Scan(&st.Task, &st.Branch, &st.Phase, &st.Container, &st.Escalation, &st.NotesLeft)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("get state %s: %w", agent, err)
	}
	return st, nil
}

// SetState writes an agent's workflow state, leaving the escalation alone: callers build a fresh
// AgentState from the columns they care about, so writing that one from the struct would clear a live
// escalation on the next phase change. SetEscalation and ClearEscalation are its only writers.
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

// SetPhase changes only an agent's phase, leaving task, branch and container as they were — skipping
// the read-then-echo SetState forces is how a held container got dropped at four call sites. It needs
// an existing row (SetState creates those), and errors rather than quietly writing nothing.
func (p *ProjectStore) SetPhase(agent, phase string) error {
	res, err := p.s.db.Exec(`UPDATE agent_state SET phase=? WHERE project=? AND agent=?`, phase, p.project, agent)
	if err != nil {
		return fmt.Errorf("set phase %s: %w", agent, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("set phase %s: %w", agent, err)
	} else if n == 0 {
		return fmt.Errorf("set phase %s: no existing state row (use SetState first)", agent)
	}
	return nil
}

// SetEscalation records the question an agent stopped on, durably: one that evaporates leaves the
// agent silently stuck. An upsert, since an agent may escalate before anything wrote it a state row.
func (p *ProjectStore) SetEscalation(agent, question string) error {
	_, err := p.s.db.Exec(`
		INSERT INTO agent_state (project,agent,escalation) VALUES (?,?,?)
		ON CONFLICT(project,agent) DO UPDATE SET escalation=excluded.escalation`,
		p.project, agent, question)
	if err != nil {
		return fmt.Errorf("set escalation %s: %w", agent, err)
	}
	return nil
}

// GrantNotes gives an agent its note budget for a claim. It REPLACES rather than adds: finishing a
// claim with two unspent and starting the next at four is what turns any quota into an occasional
// flood. Called where a claim is made, so the right to speak follows having been somewhere and looked.
func (p *ProjectStore) GrantNotes(agent string, n int) error {
	_, err := p.s.db.Exec(`
		INSERT INTO agent_state (project,agent,notes_left) VALUES (?,?,?)
		ON CONFLICT(project,agent) DO UPDATE SET notes_left=excluded.notes_left`, p.project, agent, n)
	if err != nil {
		return fmt.Errorf("grant notes to %s: %w", agent, err)
	}
	return nil
}

// SetNotesLeft records what an agent has left after spending one.
func (p *ProjectStore) SetNotesLeft(agent string, n int) error {
	_, err := p.s.db.Exec(`UPDATE agent_state SET notes_left=? WHERE project=? AND agent=?`, n, p.project, agent)
	if err != nil {
		return fmt.Errorf("set notes left for %s: %w", agent, err)
	}
	return nil
}

// NotesLeft is how many notes an agent may still send on this claim. An agent with no state row has
// never claimed anything, so it has nothing granted — the budget fails CLOSED, which is the safe
// direction for a limit whose purpose is protecting one person's attention.
func (p *ProjectStore) NotesLeft(agent string) (int, error) {
	var n int
	err := p.s.db.QueryRow(`SELECT notes_left FROM agent_state WHERE project=? AND agent=?`,
		p.project, agent).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("notes left for %s: %w", agent, err)
	}
	return n, nil
}

// ClearEscalation releases an escalated agent, whoever asked for it — the agent itself once it has
// its answer, or the user, who must be able to clear one nobody else can.
func (p *ProjectStore) ClearEscalation(agent string) error {
	_, err := p.s.db.Exec(`UPDATE agent_state SET escalation='' WHERE project=? AND agent=?`, p.project, agent)
	if err != nil {
		return fmt.Errorf("clear escalation %s: %w", agent, err)
	}
	return nil
}

// PutPR inserts or updates a merge-intent in this project. updated_at is stamped here,
// unconditionally, on every call — the caller's own value (if any) is never trusted, mirroring
// updateOwned's "any write is a change" rule for tasks, which the active filter relies on.
func (p *ProjectStore) PutPR(pr PR) error {
	if pr.CreatedAt == "" {
		pr.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if pr.Status == "" {
		pr.Status = "open"
	}
	if pr.Kind == "" {
		pr.Kind = "final"
	}
	pr.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	// The status stamp is decided in SQL against the row already stored, so it compares with what is
	// actually there rather than with whatever the caller read some steps earlier — every writer here
	// does Get, mutate, Put, and two of those interleaving would otherwise lose a transition.
	pr.StatusChangedAt = pr.UpdatedAt
	_, err := p.s.db.Exec(`
		INSERT INTO prs (project,id,task,agent,branch,base,status,feedback,created_at,kind,updated_at,status_changed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project,id) DO UPDATE SET
			task=excluded.task, agent=excluded.agent, branch=excluded.branch,
			base=excluded.base, status=excluded.status, feedback=excluded.feedback, kind=excluded.kind,
			updated_at=excluded.updated_at,
			status_changed_at=CASE WHEN prs.status<>excluded.status
				THEN excluded.status_changed_at ELSE prs.status_changed_at END`,
		p.project, pr.ID, pr.Task, pr.Agent, pr.Branch, pr.Base, pr.Status, pr.Feedback, pr.CreatedAt, pr.Kind, pr.UpdatedAt, pr.StatusChangedAt)
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

const prCols = `SELECT project,id,task,agent,branch,base,status,feedback,created_at,kind,updated_at,status_changed_at FROM prs`

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
	err := row.Scan(&p.Project, &p.ID, &p.Task, &p.Agent, &p.Branch, &p.Base, &p.Status, &p.Feedback, &p.CreatedAt, &p.Kind, &p.UpdatedAt, &p.StatusChangedAt)
	return p, err
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
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

const reviewCols = `SELECT id,pr,requirement,author,verdict,result,created_at,review_at,verdict_at,advisory FROM reviews`

// AddReview attaches a requirement to a PR in this project, unassigned. Returns id.
func (p *ProjectStore) AddReview(pr, requirement string) (int64, error) {
	res, err := p.s.db.Exec(`INSERT INTO reviews (project, pr, requirement, created_at) VALUES (?,?,?,?)`,
		p.project, pr, requirement, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("add review: %w", err)
	}
	return res.LastInsertId()
}

// AddVerdict records a completed review directly, author and verdict together — for a verdict
// that never went through the assign flow: a human's approve/reject, or a planner's advisory
// badge. Returns the new row's id.
func (p *ProjectStore) AddVerdict(pr, requirement, author, verdict, result string, advisory bool) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := p.s.db.Exec(
		`INSERT INTO reviews (project, pr, requirement, author, review_at, verdict, result, verdict_at, advisory, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.project, pr, requirement, author, now, verdict, result, now, advisory, now)
	if err != nil {
		return 0, fmt.Errorf("add verdict: %w", err)
	}
	return res.LastInsertId()
}

// ApprovalCounts maps each PR in this project to how many approvals it has accumulated —
// reviewer, human and planner badges alike, since this is what a person sees, not what gates
// the merge (-> api.PRApprovable, which reads pr.Status instead).
func (p *ProjectStore) ApprovalCounts() (map[string]int, error) {
	rows, err := p.s.db.Query(
		`SELECT pr, COUNT(*) FROM reviews WHERE project=? AND verdict='pass' GROUP BY pr`, p.project)
	if err != nil {
		return nil, fmt.Errorf("approval counts: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var pr string
		var n int
		if err := rows.Scan(&pr, &n); err != nil {
			return nil, err
		}
		out[pr] = n
	}
	return out, rows.Err()
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

// UnclaimedReview returns the oldest review nobody is doing, for a PR that is still open — what a
// reviewer picks up when it finishes one and is free again, and what covers a review requested
// while no reviewer was running. (0, "", false) when there is none.
func (p *ProjectStore) UnclaimedReview(id *int64, pr *string) (bool, error) {
	err := p.s.db.QueryRow(`
		SELECT r.id, r.pr FROM reviews r JOIN prs pp ON pp.project=r.project AND pp.id=r.pr
		WHERE r.project=? AND r.author='' AND r.verdict='' AND pp.status='open'
		ORDER BY r.id LIMIT 1`, p.project).Scan(id, pr)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("unclaimed review: %w", err)
	}
	return true, nil
}

// ActiveReviewers maps each PR in this project to the agent holding an open review of it. One query
// for the whole project, since every PR list wants it and a lookup per row would be paid per render.
func (p *ProjectStore) ActiveReviewers() (map[string]string, error) {
	rows, err := p.s.db.Query(
		`SELECT pr, author FROM reviews WHERE project=? AND verdict='' AND author!='' ORDER BY id`,
		p.project)
	if err != nil {
		return nil, fmt.Errorf("active reviewers: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var pr, author string
		if err := rows.Scan(&pr, &author); err != nil {
			return nil, err
		}
		out[pr] = author
	}
	return out, rows.Err()
}

// AmendReview replaces an open review's requirement, for a second instruction arriving while the
// first is still being carried out. The review is the same one: the reviewer keeps the branch it
// has checked out and is simply told more.
func (p *ProjectStore) AmendReview(id int64, requirement string) error {
	_, err := p.s.db.Exec(`UPDATE reviews SET requirement=? WHERE id=? AND project=?`,
		requirement, id, p.project)
	if err != nil {
		return fmt.Errorf("amend review %d: %w", id, err)
	}
	return nil
}

// CloseReviews ends every open review of a PR without a verdict — what a merge or a scrap does to a
// review that has been overtaken: the thing it was about is settled, so nobody should still hold it.
func (p *ProjectStore) CloseReviews(pr, why string) error {
	_, err := p.s.db.Exec(
		`UPDATE reviews SET verdict='moot', result=? WHERE project=? AND pr=? AND verdict=''`,
		why, p.project, pr)
	if err != nil {
		return fmt.Errorf("close reviews of %s: %w", pr, err)
	}
	return nil
}

// LiveReviewPRs is the set of PR ids in this project carrying at least one unverdicted review row
// — held or unclaimed. One query for the whole project (-> ActiveReviewers, ApprovalCounts), since
// the review-row invariant sweeps every open PR and a query per PR would be paid per one of them.
func (p *ProjectStore) LiveReviewPRs() (map[string]bool, error) {
	rows, err := p.s.db.Query(`SELECT DISTINCT pr FROM reviews WHERE project=? AND verdict=''`, p.project)
	if err != nil {
		return nil, fmt.Errorf("live review prs: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var pr string
		if err := rows.Scan(&pr); err != nil {
			return nil, err
		}
		out[pr] = true
	}
	return out, rows.Err()
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
			&r.Result, &r.CreatedAt, &r.ReviewAt, &r.VerdictAt, &r.Advisory); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RuledPRs is every PR author has recorded a verdict on, newest first — what a reviewer may still
// comment on, its HELD review having ended the moment that verdict landed (-> ReviewingPR).
func (p *ProjectStore) RuledPRs(author string) ([]string, error) {
	rows, err := p.s.db.Query(
		`SELECT pr FROM reviews WHERE project=? AND author=? AND verdict<>'' ORDER BY id DESC`,
		p.project, author)
	if err != nil {
		return nil, fmt.Errorf("ruled prs for %s: %w", author, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var pr string
		if err := rows.Scan(&pr); err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// AwaitingPR is agent's newest PR that has not settled, with the task it would land into — ("", "")
// if none. An agent HOLDS that task until the PR merges, which is what the board has always shown
// and what every "is it free" question in the hub used to miss.
func (p *ProjectStore) AwaitingPR(agent string) (pr, task string, err error) {
	err = p.s.db.QueryRow(
		`SELECT id, task FROM prs WHERE project=? AND agent=? AND status NOT IN ('merged','scrapped') ORDER BY rowid DESC LIMIT 1`,
		p.project, agent).Scan(&pr, &task)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("awaiting pr for %s: %w", agent, err)
	}
	return pr, task, nil
}

// ReviewingPR is the newest verdict-less review assigned to author, "" if none. The board
// needs it because a reviewer authors no PR, leaving its AgentView.PR empty.
func (p *ProjectStore) ReviewingPR(author string) (string, error) {
	var pr string
	err := p.s.db.QueryRow(
		`SELECT pr FROM reviews WHERE project=? AND author=? AND verdict='' ORDER BY id DESC LIMIT 1`,
		p.project, author).Scan(&pr)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reviewing pr for %s: %w", author, err)
	}
	return pr, nil
}
