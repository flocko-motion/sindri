// package: hub/store / store
// type:    persistence (SQLite, hub-owned)
// job:     the global hub's durable source of truth — roster + activity log — in
//          one central SQLite DB. Every per-repo row is tagged by a `project` key;
//          a `*Store` owns the DB and cross-project reads, and `Store.For(project)`
//          returns a project-scoped `*ProjectStore`.
// limits:  single-owner (only the hub touches it); SQLite is a linked library,
//          not an external tool, so this is NOT an internal/adapter package.
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

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
}

// Event is one row of the append-only activity log.
type Event struct {
	ID      int64  `json:"id"`
	Project string `json:"project"`
	Agent   string `json:"agent"`
	TS      string `json:"ts"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// Project is one row of the registry: a repo the hub knows, keyed by its stable
// repoTag (a digest of the abs path), with the on-disk path, when first seen, and
// when last used (touched on every register/use, so the repo switcher can order by
// recency).
type Project struct {
	Tag       string `json:"tag"`
	Path      string `json:"path"`
	FirstSeen string `json:"first_seen"`
	LastUsed  string `json:"last_used"`
}

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
  last_used  TEXT NOT NULL DEFAULT ''
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
	if _, err := db.Exec(schema + workflowSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// migrate adds columns that CREATE TABLE IF NOT EXISTS can't add to a pre-existing
// table. Each ALTER is idempotent — a "duplicate column" error means it's already
// applied and is ignored; any other error is real.
func migrate(db *sql.DB) error {
	alters := []string{
		`ALTER TABLE agents ADD COLUMN memory TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE projects ADD COLUMN last_used TEXT NOT NULL DEFAULT ''`,
	}
	for _, a := range alters {
		if _, err := db.Exec(a); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("migrate (%s): %w", a, err)
		}
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
	rows, err := s.db.Query(`SELECT tag, path, first_seen, last_used FROM projects ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("projects: %w", err)
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.Tag, &p.Path, &p.FirstSeen, &p.LastUsed); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
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
		`SELECT project, name, role, workspace, socket, created_at, memory FROM agents ORDER BY project, name`)
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
		if err := rows.Scan(&a.Project, &a.Name, &a.Role, &a.Workspace, &a.Socket, &a.CreatedAt, &a.Memory); err != nil {
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
		INSERT INTO agents (project, name, role, workspace, socket, created_at, memory)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project, name) DO UPDATE SET
			role=excluded.role, workspace=excluded.workspace, socket=excluded.socket, memory=excluded.memory`,
		a.Project, a.Name, a.Role, a.Workspace, a.Socket, a.CreatedAt, a.Memory)
	if err != nil {
		return fmt.Errorf("put agent %s/%s: %w", a.Project, a.Name, err)
	}
	return nil
}

// GetAgent returns an agent by name within this project; ok is false if absent.
func (p *ProjectStore) GetAgent(name string) (a Agent, ok bool, err error) {
	row := p.s.db.QueryRow(
		`SELECT project, name, role, workspace, socket, created_at, memory FROM agents WHERE project=? AND name=?`,
		p.project, name)
	err = row.Scan(&a.Project, &a.Name, &a.Role, &a.Workspace, &a.Socket, &a.CreatedAt, &a.Memory)
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
		`SELECT project, name, role, workspace, socket, created_at, memory FROM agents WHERE project=? ORDER BY name`,
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
	return nil
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
