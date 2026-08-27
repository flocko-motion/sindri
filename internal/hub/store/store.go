// package: hub/store / store
// type:    adapter (SQLite, hub-owned)
// job:     the global hub's durable source of truth — roster + activity log — in
// one central SQLite DB. Every per-repo row is tagged by a `project` key;
// a `*Store` owns the DB and cross-project reads, and `Store.For(project)`
// returns a project-scoped `*ProjectStore`.
// limits:  single-owner (only the hub touches it); wraps the external SQLite
// store, holding no domain rules of its own.
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	_ "modernc.org/sqlite"
)

// Agent is one roster row: an agent's durable identity, unique per (project, name).
type Agent struct {
	Project   string `json:"project"`
	Name      string `json:"name"`
	Role      string `json:"role"` // "worker" | "reviewer" | "planner" | "coauthor"
	Workspace string `json:"workspace"`
	Socket    string `json:"socket"`
	CreatedAt string `json:"created_at"`
	Memory    string `json:"memory"` // per-agent RAM limit (e.g. "4g"); "" = hub default
	// Retired: hand it no NEW work. What it already holds it finishes, so this is how an agent is
	// wound down without interrupting it — the flag a human sets, distinct from the automatic
	// retirement a full context causes.
	Retired bool `json:"retired"`
	// ClearArmed: a human has armed a context clear, which fires at the agent's next leaf boundary.
	// Durable because the hub may restart between the arming and the boundary, and an arming that
	// evaporated would leave the human believing it was set.
	ClearArmed bool `json:"clear_armed"`
	// Stopped: a human tore the pod down on purpose, distinct from a crash — set once StopAgent's
	// removal succeeds, cleared once Launch is asked to bring it back. Durable so a hub restart
	// between the two still tells "stopped" apart from "down".
	Stopped bool `json:"stopped"`
	// Model is the model this agent launches on, "" for the account default. A property of the
	// agent, like its role — set at creation or by SetModel, and authoritative while the agent is
	// down or stopped, since there is no live session to read one off instead.
	Model string `json:"model"`
}

// Event is one row of the append-only activity log; it crosses the wire, so it is
// internal/api.Event under the name every existing caller here already uses.
type Event = api.Event

// Project is one row of the registry; it crosses the wire, so it is
// internal/api.Project under the name every existing caller here already uses.
type Project = api.Project

// Store wraps the one central SQLite database. Per-project work goes through a
// ProjectStore from For; cross-project reads and the registry live here.
type Store struct {
	db *sql.DB
}

// ProjectStore is a project-scoped view over the one Store: every method it exposes
// implicitly filters/tags by its project, so callers never thread a project id.
type ProjectStore struct {
	s       *Store
	project string
}

const schema = `
CREATE TABLE IF NOT EXISTS agents (
  project    TEXT NOT NULL,
  name       TEXT NOT NULL,
  role       TEXT NOT NULL,
  workspace  TEXT NOT NULL DEFAULT '',
  socket     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  memory     TEXT NOT NULL DEFAULT '',
  retired    INTEGER NOT NULL DEFAULT 0,
  clear_armed INTEGER NOT NULL DEFAULT 0,
  stopped    INTEGER NOT NULL DEFAULT 0,
  model      TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, name)
);
CREATE TABLE IF NOT EXISTS events (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  project TEXT NOT NULL,
  agent   TEXT NOT NULL,
  ts      TEXT NOT NULL,
  type    TEXT NOT NULL,
  payload TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_events_agent ON events(project, agent, id);
-- Hub-global key/value (not per-project): the agent-token secret, the TCP port, …
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
-- The registry of repos the hub serves: repoTag -> path.
CREATE TABLE IF NOT EXISTS projects (
  tag        TEXT PRIMARY KEY,
  path       TEXT NOT NULL,
  first_seen TEXT NOT NULL,
  last_used  TEXT NOT NULL DEFAULT '',
  color      INTEGER NOT NULL DEFAULT 0
);
-- The user's single chatroom (star topology): which agents are members, and the
-- transcript of everything the hub has forwarded. Hub-global (one room), so no
-- project column on the log; members carry (project, name) since agents are
-- project-scoped.
CREATE TABLE IF NOT EXISTS chat_members (
  project  TEXT NOT NULL,
  name     TEXT NOT NULL,
  added_at TEXT NOT NULL,
  PRIMARY KEY (project, name)
);
CREATE TABLE IF NOT EXISTS chat_log (
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  sender TEXT NOT NULL,
  body   TEXT NOT NULL,
  ts     TEXT NOT NULL
);
-- Unified task comments, synced from external sources (td, github). source + a
-- source_ref (the external id/url) key each comment so a re-sync reconciles them
-- against their origin (add new, drop removed, update changed).
CREATE TABLE IF NOT EXISTS task_comments (
  project    TEXT NOT NULL,
  task_id    TEXT NOT NULL,
  source     TEXT NOT NULL, -- "github", or "td" on a thread synced before the import
  source_ref TEXT NOT NULL, -- external id/url, unique within a source
  author     TEXT NOT NULL DEFAULT '',
  body       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (project, task_id, source, source_ref)
);
`

// Open opens (creating if needed) the central SQLite DB at path and applies the
// schema. WAL mode keeps reads concurrent with the single writer.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // single writer; serialise to avoid SQLITE_BUSY
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}
	if _, err := db.Exec(schema + workflowSchema + mailSchema + mailLogSchema + submitGateSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	// state_log is debug telemetry, never durable across a restart (-> its own schema comment): a hub
	// up for weeks must not hold weeks of flicker just because nothing else trims it.
	if _, err := db.Exec(`DELETE FROM state_log`); err != nil {
		db.Close()
		return nil, fmt.Errorf("purge state log: %w", err)
	}
	return &Store{db: db}, nil
}

// migrate adds columns that CREATE TABLE IF NOT EXISTS can't add to a pre-existing
// table. Each ALTER is idempotent — a "duplicate column" error means it's already
// applied and is ignored; any other error is real.
func migrate(db *sql.DB) error {
	alters := []string{
		`ALTER TABLE agents ADD COLUMN memory TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN retired INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN clear_armed INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN stopped INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN model TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE projects ADD COLUMN last_used TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE projects ADD COLUMN color INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE tasks ADD COLUMN description TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN created_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN tier TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE owned_tasks ADD COLUMN tier TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE prs ADD COLUMN kind TEXT NOT NULL DEFAULT 'final'`,
		`ALTER TABLE reviews ADD COLUMN advisory INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE prs ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE prs ADD COLUMN status_changed_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agent_state ADD COLUMN escalation TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agent_state ADD COLUMN notes_left INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agent_state ADD COLUMN last_nudge TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE mail ADD COLUMN in_reply_to INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE mail ADD COLUMN notified INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE pr_lint ADD COLUMN sha TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE runs ADD COLUMN commit_sha TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE submit_answers ADD COLUMN done INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE task_priority ADD COLUMN tier TEXT NOT NULL DEFAULT ''`,
	}
	for _, a := range alters {
		if _, err := db.Exec(a); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("migrate (%s): %w", a, err)
		}
	}
	// Parentage moved out of owned_tasks into task_parent, which holds it for every task. Carry the
	// links a store written before that still has in the old column; a database created since has
	// no such column, and the copy simply finds nothing to do.
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO task_parent (project,id,parent_id)
		 SELECT project,id,parent_id FROM owned_tasks WHERE parent_id != ''`); err != nil &&
		!strings.Contains(err.Error(), "no such column") {
		return fmt.Errorf("migrate (parentage): %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// For returns a project-scoped handle over this store.
func (s *Store) For(project string) *ProjectStore { return &ProjectStore{s: s, project: project} }

// --- registry (global) ---

// RegisterProject records (or refreshes the path of) a repo the hub now serves, and
// touches its last_used stamp — this is called on every request that carries a repo,
// so last_used doubles as the "recently active" signal the switcher orders by.
func (s *Store) RegisterProject(tag, path string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO projects (tag, path, first_seen, last_used) VALUES (?, ?, ?, ?)
		 ON CONFLICT(tag) DO UPDATE SET path=excluded.path, last_used=excluded.last_used`,
		tag, path, now, now)
	if err != nil {
		return fmt.Errorf("register project %s: %w", tag, err)
	}
	return nil
}

// UnregisterProject removes a repo's registry row and nothing else — no agents,
// events, tasks, or on-disk files are touched. This backs `repo forget`: the hub
// gives up tracking the repo, it does not delete it (and implicit registration
// re-adds it on next use).
func (s *Store) UnregisterProject(tag string) error {
	if _, err := s.db.Exec(`DELETE FROM projects WHERE tag=?`, tag); err != nil {
		return fmt.Errorf("unregister project %s: %w", tag, err)
	}
	return nil
}

// ProjectPath resolves a repoTag to its on-disk path; ok is false if unknown. A
// real query error is returned (distinct from "unknown project"), never collapsed
// into a silent "" that would hand callers the wrong repo root.
func (s *Store) ProjectPath(tag string) (path string, ok bool, err error) {
	err = s.db.QueryRow(`SELECT path FROM projects WHERE tag=?`, tag).Scan(&path)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("project path %s: %w", tag, err)
	}
	return path, true, nil
}

// Projects returns every known project, ordered by path. Callers that need recency
// or live-agent ordering (the switcher) re-sort using LastUsed and the roster.
func (s *Store) Projects() ([]Project, error) {
	rows, err := s.db.Query(`SELECT tag, path, first_seen, last_used, color FROM projects ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("projects: %w", err)
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.Tag, &p.Path, &p.FirstSeen, &p.LastUsed, &p.Color); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetProjectColor sets a repo's colour choice (0 = hash-derived default, 1..N = a
// palette index). A no-op-safe write on an unknown tag (affects zero rows).
func (s *Store) SetProjectColor(tag string, color int) error {
	if _, err := s.db.Exec(`UPDATE projects SET color=? WHERE tag=?`, color, tag); err != nil {
		return fmt.Errorf("set project colour %s: %w", tag, err)
	}
	return nil
}

// --- meta (global) ---

// GetMeta returns a hub-global key/value (ok=false when unset).
func (s *Store) GetMeta(key string) (value string, ok bool, err error) {
	row := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, key)
	err = row.Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get meta %s: %w", key, err)
	}
	return value, true, nil
}

// SetMeta inserts or updates a hub-global key/value.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value)
	if err != nil {
		return fmt.Errorf("set meta %s: %w", key, err)
	}
	return nil
}

// --- agents (global read) ---

// AllAgents returns every agent across all projects, ordered by (project, name) —
// the canonical set backing the global board and token resolution.
func (s *Store) AllAgents() ([]Agent, error) {
	rows, err := s.db.Query(
		`SELECT project, name, role, workspace, socket, created_at, memory, retired, clear_armed, stopped, model FROM agents ORDER BY project, name`)
	if err != nil {
		return nil, fmt.Errorf("all agents: %w", err)
	}
	defer rows.Close()
	return scanAgents(rows)
}

func scanAgents(rows *sql.Rows) ([]Agent, error) {
	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.Project, &a.Name, &a.Role, &a.Workspace, &a.Socket, &a.CreatedAt, &a.Memory, &a.Retired, &a.ClearArmed, &a.Stopped, &a.Model); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// --- agents (project-scoped) ---

// PutAgent inserts or updates an agent in this project, preserving created_at.
func (p *ProjectStore) PutAgent(a Agent) error {
	a.Project = p.project
	if a.CreatedAt == "" {
		a.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := p.s.db.Exec(`
		INSERT INTO agents (project, name, role, workspace, socket, created_at, memory, retired, clear_armed, stopped, model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project, name) DO UPDATE SET
			role=excluded.role, workspace=excluded.workspace, socket=excluded.socket,
			memory=excluded.memory, retired=excluded.retired, clear_armed=excluded.clear_armed,
			stopped=excluded.stopped, model=excluded.model`,
		a.Project, a.Name, a.Role, a.Workspace, a.Socket, a.CreatedAt, a.Memory, a.Retired, a.ClearArmed, a.Stopped, a.Model)
	if err != nil {
		return fmt.Errorf("put agent %s/%s: %w", a.Project, a.Name, err)
	}
	return nil
}

// GetAgent returns an agent by name within this project; ok is false if absent.
func (p *ProjectStore) GetAgent(name string) (a Agent, ok bool, err error) {
	row := p.s.db.QueryRow(
		`SELECT project, name, role, workspace, socket, created_at, memory, retired, clear_armed, stopped, model FROM agents WHERE project=? AND name=?`,
		p.project, name)
	err = row.Scan(&a.Project, &a.Name, &a.Role, &a.Workspace, &a.Socket, &a.CreatedAt, &a.Memory, &a.Retired, &a.ClearArmed, &a.Stopped, &a.Model)
	if err == sql.ErrNoRows {
		return Agent{}, false, nil
	}
	if err != nil {
		return Agent{}, false, fmt.Errorf("get agent %s/%s: %w", p.project, name, err)
	}
	return a, true, nil
}

// Roster returns this project's agents, ordered by name.
func (p *ProjectStore) Roster() ([]Agent, error) {
	rows, err := p.s.db.Query(
		`SELECT project, name, role, workspace, socket, created_at, memory, retired, clear_armed, stopped, model FROM agents WHERE project=? ORDER BY name`,
		p.project)
	if err != nil {
		return nil, fmt.Errorf("roster %s: %w", p.project, err)
	}
	defer rows.Close()
	return scanAgents(rows)
}

// DeleteAgent removes an agent (and its activity log) from this project.
func (p *ProjectStore) DeleteAgent(name string) error {
	if _, err := p.s.db.Exec(`DELETE FROM agents WHERE project=? AND name=?`, p.project, name); err != nil {
		return fmt.Errorf("delete agent %s/%s: %w", p.project, name, err)
	}
	// Drop the activity log too, so a future agent reusing this name doesn't
	// rehydrate from a stranger's history.
	if _, err := p.s.db.Exec(`DELETE FROM events WHERE project=? AND agent=?`, p.project, name); err != nil {
		return fmt.Errorf("delete agent events %s/%s: %w", p.project, name, err)
	}
	// And drop any chatroom membership, so a deleted agent doesn't linger as a
	// phantom member the hub tries to forward to.
	if _, err := p.s.db.Exec(`DELETE FROM chat_members WHERE project=? AND name=?`, p.project, name); err != nil {
		return fmt.Errorf("delete agent chat membership %s/%s: %w", p.project, name, err)
	}
	return nil
}

// AgentsNamed is every agent with this name, across every project. Global uniqueness is the
// allocator's convention (AutoName checks the whole fleet) and NOT a schema constraint — mailboxes are
// keyed (project, name) — so a caller addressing an agent by bare name gets every candidate and
// decides. Delivering to the wrong dvalin is the one failure here worth engineering against.
func (s *Store) AgentsNamed(name string) ([]Agent, error) {
	rows, err := s.db.Query(
		`SELECT project, name, role, workspace, socket, created_at, memory, retired, clear_armed, stopped, model FROM agents WHERE name=? ORDER BY project`,
		name)
	if err != nil {
		return nil, fmt.Errorf("agents named %q: %w", name, err)
	}
	defer rows.Close()
	return scanAgents(rows)
}

// Log appends an activity-log entry for an agent in this project.
func (p *ProjectStore) Log(agent, typ, payload string) error {
	_, err := p.s.db.Exec(
		`INSERT INTO events (project, agent, ts, type, payload) VALUES (?, ?, ?, ?, ?)`,
		p.project, agent, time.Now().UTC().Format(time.RFC3339), typ, payload)
	if err != nil {
		return fmt.Errorf("log event for %s/%s: %w", p.project, agent, err)
	}
	return nil
}

// Events returns an agent's most recent events in this project, oldest-first,
// capped at limit (limit <= 0 means all).
func (p *ProjectStore) Events(agent string, limit int) ([]Event, error) {
	q := `SELECT id, project, agent, ts, type, payload FROM events WHERE project=? AND agent=? ORDER BY id DESC`
	args := []any{p.project, agent}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := p.s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("events for %s/%s: %w", p.project, agent, err)
	}
	defer rows.Close()
	var evs []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Project, &e.Agent, &e.TS, &e.Type, &e.Payload); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		evs = append(evs, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse to oldest-first so a rehydrate briefing reads chronologically.
	for i, j := 0, len(evs)-1; i < j; i, j = i+1, j-1 {
		evs[i], evs[j] = evs[j], evs[i]
	}
	return evs, nil
}
