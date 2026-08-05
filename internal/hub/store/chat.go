// package: hub/store / chat
// type:    logic (persistence for the user's chatroom)
// job:     store chatroom membership (which agents are in the room) and the room
// transcript (every message the hub has forwarded), hub-global — there is
// exactly one room. Reads/writes only; the relay + delivery live in the hub.
// limits:  no delivery, no tmux, no HTTP — just rows.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/flo-at/sindri/internal/api"
)

// ChatMember is one agent in the chatroom; it crosses the wire, so it is
// internal/api.ChatMember under the name every existing caller here already uses.
type ChatMember = api.ChatMember

// ChatMessage is one line of the room transcript; it crosses the wire, so it is
// internal/api.ChatMessage under the name every existing caller here already uses.
type ChatMessage = api.ChatMessage

// ChatAdd adds an agent to the chatroom. Idempotent: re-adding an existing member
// is a no-op (the caller checks ChatIsMember first to decide whether to greet).
func (s *Store) ChatAdd(project, name string) error {
	_, err := s.db.Exec(
		`INSERT INTO chat_members (project, name, added_at) VALUES (?, ?, ?)
		 ON CONFLICT(project, name) DO NOTHING`,
		project, name, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("chat add %s/%s: %w", project, name, err)
	}
	return nil
}

// ChatRemove removes an agent from the chatroom, reporting whether it had been a
// member (so the caller only announces a real removal).
func (s *Store) ChatRemove(project, name string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM chat_members WHERE project=? AND name=?`, project, name)
	if err != nil {
		return false, fmt.Errorf("chat remove %s/%s: %w", project, name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("chat remove %s/%s: %w", project, name, err)
	}
	return n > 0, nil
}

// ChatIsMember reports whether an agent is currently in the chatroom.
func (s *Store) ChatIsMember(project, name string) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM chat_members WHERE project=? AND name=?`, project, name).Scan(&one)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("chat is-member %s/%s: %w", project, name, err)
	}
	return true, nil
}

// ChatMembers returns every agent in the chatroom, ordered by (project, name),
// each with its role joined from the agents table.
func (s *Store) ChatMembers() ([]ChatMember, error) {
	rows, err := s.db.Query(
		`SELECT m.project, m.name, COALESCE(a.role, '')
		   FROM chat_members m
		   LEFT JOIN agents a ON a.project = m.project AND a.name = m.name
		  ORDER BY m.project, m.name`)
	if err != nil {
		return nil, fmt.Errorf("chat members: %w", err)
	}
	defer rows.Close()
	var out []ChatMember
	for rows.Next() {
		var m ChatMember
		if err := rows.Scan(&m.Project, &m.Name, &m.Role); err != nil {
			return nil, fmt.Errorf("scan chat member: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ChatAppend records a forwarded message in the transcript and returns its row.
func (s *Store) ChatAppend(sender, body string) (ChatMessage, error) {
	ts := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO chat_log (sender, body, ts) VALUES (?, ?, ?)`, sender, body, ts)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("chat append: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ChatMessage{}, fmt.Errorf("chat append: %w", err)
	}
	return ChatMessage{ID: id, Sender: sender, Body: body, TS: ts}, nil
}

// ChatClearTranscript deletes the room transcript, leaving membership alone: a new meeting resets
// the shared history, not who is in the room. Reports how many lines were dropped, so the caller
// can say whether there was anything to clear.
func (s *Store) ChatClearTranscript() (int, error) {
	res, err := s.db.Exec(`DELETE FROM chat_log`)
	if err != nil {
		return 0, fmt.Errorf("chat clear: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("chat clear: %w", err)
	}
	return int(n), nil
}

// ChatTranscript returns the most recent limit messages in chronological order
// (oldest first). A non-positive limit returns the whole transcript.
func (s *Store) ChatTranscript(limit int) ([]ChatMessage, error) {
	q := `SELECT id, sender, body, ts FROM chat_log ORDER BY id`
	args := []any{}
	if limit > 0 {
		// Take the newest `limit`, then re-sort ascending so callers get chronology.
		q = `SELECT id, sender, body, ts FROM (
		       SELECT id, sender, body, ts FROM chat_log ORDER BY id DESC LIMIT ?
		     ) ORDER BY id`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("chat transcript: %w", err)
	}
	defer rows.Close()
	var out []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.Sender, &m.Body, &m.TS); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
