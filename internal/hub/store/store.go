// package: hub/store / store
// type:    persistence (SQLite, hub-owned)
// job:     the hub's durable single source of truth — the agent roster and the
//          append-only per-agent activity log — in one SQLite DB at
//          `.sindri/hub.db`. Every mutation is a transaction so a crash loses
//          nothing committed.
// limits:  single-owner (only the hub touches it); SQLite is a linked library,
//          not an external tool, so this is NOT an internal/adapter package.
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Agent is one roster row: an agent's durable identity. Live workflow state
// (task, verdict) is added in later phases; Phase 1 records identity only.
type Agent struct {
	Name      string `json:"name"`
	Role      string `json:"role"` // "worker" | "reviewer"
	Workspace string `json:"workspace"`
	Socket    string `json:"socket"`
	CreatedAt string `json:"created_at"`
}

// Event is one row of the append-only activity log.
type Event struct {
	ID      int64  `json:"id"`
	Agent   string `json:"agent"`
	TS      string `json:"ts"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS agents (
  name       TEXT PRIMARY KEY,
  role       TEXT NOT NULL,
  workspace  TEXT NOT NULL DEFAULT '',
  socket     TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  agent   TEXT NOT NULL,
  ts      TEXT NOT NULL,
  type    TEXT NOT NULL,
  payload TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_events_agent ON events(agent, id);
`

// Open opens (creating if needed) the SQLite DB at path and applies the schema.
// WAL mode keeps reads concurrent with the single writer.
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
	migrate(db) // additive column migrations for DBs created by an older schema
	return &Store{db: db}, nil
}

// migrate applies additive column migrations to THIS store's own database
// (`.sindri/hub.db`, hub-owned) for DBs created by an older schema. It never
// touches td's database — td's `issues` table is read-only here (direct SELECT;
// writes go through the td CLI). The table below is the hub's own `tasks` cache,
// not td's issues. `CREATE TABLE IF NOT EXISTS` won't alter an existing table, so
// new columns are added idempotently (a duplicate-column error is ignored).
func migrate(db *sql.DB) {
	for _, stmt := range []string{
		`ALTER TABLE tasks ADD COLUMN parent_id TEXT NOT NULL DEFAULT ''`, // hub.db cache
	} {
		_, _ = db.Exec(stmt) // ignore "duplicate column name" on already-migrated DBs
	}
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// PutAgent inserts or updates an agent, preserving created_at across updates.
func (s *Store) PutAgent(a Agent) error {
	if a.CreatedAt == "" {
		a.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`
		INSERT INTO agents (name, role, workspace, socket, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			role=excluded.role, workspace=excluded.workspace, socket=excluded.socket`,
		a.Name, a.Role, a.Workspace, a.Socket, a.CreatedAt)
	if err != nil {
		return fmt.Errorf("put agent %s: %w", a.Name, err)
	}
	return nil
}

// GetAgent returns an agent by name; ok is false if it does not exist.
func (s *Store) GetAgent(name string) (a Agent, ok bool, err error) {
	row := s.db.QueryRow(
		`SELECT name, role, workspace, socket, created_at FROM agents WHERE name=?`, name)
	err = row.Scan(&a.Name, &a.Role, &a.Workspace, &a.Socket, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return Agent{}, false, nil
	}
	if err != nil {
		return Agent{}, false, fmt.Errorf("get agent %s: %w", name, err)
	}
	return a, true, nil
}

// Roster returns every agent, ordered by name — the canonical set of agents.
func (s *Store) Roster() ([]Agent, error) {
	rows, err := s.db.Query(
		`SELECT name, role, workspace, socket, created_at FROM agents ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("roster: %w", err)
	}
	defer rows.Close()
	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.Name, &a.Role, &a.Workspace, &a.Socket, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// DeleteAgent removes an agent from the roster.
func (s *Store) DeleteAgent(name string) error {
	if _, err := s.db.Exec(`DELETE FROM agents WHERE name=?`, name); err != nil {
		return fmt.Errorf("delete agent %s: %w", name, err)
	}
	// Drop the activity log too, so a future agent that reuses this (auto-named)
	// name doesn't rehydrate from a stranger's history.
	if _, err := s.db.Exec(`DELETE FROM events WHERE agent=?`, name); err != nil {
		return fmt.Errorf("delete agent events %s: %w", name, err)
	}
	return nil
}

// Log appends an activity-log entry for an agent. This is the spine of the
// per-worker timeline; it records only hub-mediated interaction.
func (s *Store) Log(agent, typ, payload string) error {
	_, err := s.db.Exec(
		`INSERT INTO events (agent, ts, type, payload) VALUES (?, ?, ?, ?)`,
		agent, time.Now().UTC().Format(time.RFC3339), typ, payload)
	if err != nil {
		return fmt.Errorf("log event for %s: %w", agent, err)
	}
	return nil
}

// Events returns an agent's most recent events, oldest-first, capped at limit
// (limit <= 0 means all). Used by rehydrate (D13) and the TUI timeline (D12).
func (s *Store) Events(agent string, limit int) ([]Event, error) {
	q := `SELECT id, agent, ts, type, payload FROM events WHERE agent=? ORDER BY id DESC`
	args := []any{agent}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("events for %s: %w", agent, err)
	}
	defer rows.Close()
	var evs []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Agent, &e.TS, &e.Type, &e.Payload); err != nil {
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
