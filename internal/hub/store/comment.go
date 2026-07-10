// package: hub/store / comment
// type:    logic (persistence for unified task comments)
// job:     store task comments synced from external sources (td, github) in one
//          place, keyed by (source, source_ref) so a re-sync reconciles them
//          against their origin. Reads/writes only; fetching + reconcile live in
//          the hub, rendering in the UIs.
// limits:  no fetching, no source knowledge — just rows keyed for sync.
package store

import "fmt"

// Comment is one task comment, tagged with the source it came from and the
// external reference that identifies it there (so a re-sync can match it).
type Comment struct {
	Source    string `json:"source"`     // "td" | "github"
	SourceRef string `json:"source_ref"` // external id / url, unique within the source
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"` // RFC3339
}

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
