// package: hub/store / comment
// type:    logic (persistence for unified task comments)
// job:     store task comments synced from external sources (td, github) in one
// place, keyed by (source, source_ref) so a re-sync reconciles them
// against their origin. Reads/writes only; fetching + reconcile live in
// the hub, rendering in the UIs.
// limits:  no fetching, no source knowledge — just rows keyed for sync.
package store

import (
	"fmt"

	"github.com/flo-at/sindri/internal/api"
)

// Comment is one task comment; it crosses the wire, so it is internal/api.Comment
// under the name every existing caller here already uses.
type Comment = api.Comment

// ReplaceComments reconciles a task's comments FROM ONE SOURCE: it drops the
// source's existing comments for the task and inserts the given set, so a comment
// added at the source appears, a removed one drops, and a changed one updates —
// all from re-fetching the source's current set. Other sources are untouched.
func (p *ProjectStore) ReplaceComments(taskID, source string, comments []Comment) error {
	tx, err := p.s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM task_comments WHERE project=? AND task_id=? AND source=?`,
		p.project, taskID, source); err != nil {
		return err
	}
	for _, c := range comments {
		if _, err := tx.Exec(
			`INSERT INTO task_comments (project,task_id,source,source_ref,author,body,created_at)
			 VALUES (?,?,?,?,?,?,?)`,
			p.project, taskID, source, c.SourceRef, c.Author, c.Body, c.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddComment records one comment. Used for the sources that keep no thread of their own, where this
// table is where a comment lives rather than a cache of somewhere else.
func (p *ProjectStore) AddComment(taskID string, c Comment) error {
	_, err := p.s.db.Exec(
		`INSERT INTO task_comments (project,task_id,source,source_ref,author,body,created_at)
		 VALUES (?,?,?,?,?,?,?)`,
		p.project, taskID, c.Source, c.SourceRef, c.Author, c.Body, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("add comment on %s: %w", taskID, err)
	}
	return nil
}

// Comments returns a task's comments across all sources, oldest first.
func (p *ProjectStore) Comments(taskID string) ([]Comment, error) {
	rows, err := p.s.db.Query(
		`SELECT source, source_ref, author, body, created_at FROM task_comments
		 WHERE project=? AND task_id=? ORDER BY created_at, source_ref`, p.project, taskID)
	if err != nil {
		return nil, fmt.Errorf("comments %s: %w", taskID, err)
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.Source, &c.SourceRef, &c.Author, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteComments drops every comment for a task (used when the task is scrapped).
func (p *ProjectStore) DeleteComments(taskID string) error {
	_, err := p.s.db.Exec(`DELETE FROM task_comments WHERE project=? AND task_id=?`, p.project, taskID)
	return err
}
