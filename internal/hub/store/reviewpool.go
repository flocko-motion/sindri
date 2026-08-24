// package: hub/store / reviewpool
// type:    adapter (SQLite, hub-owned)
// job:     what a reviewer holds and has ruled on, fleet-wide — a pooled (GlobalProject) reviewer's
// rows are never filed under its own project, so every caller that cannot assume it holds a
// project-bound reviewer answers this instead of workflow.go's *ProjectStore methods.
// limits:  reads only; the rows themselves are workflow.go's (AddReview/RecordVerdict/AssignReview).
package store

import (
	"database/sql"
	"fmt"
)

// ReviewingPR is what author holds AND where it is filed — home's own project first, else
// fleet-wide: a pooled reviewer's held review is never filed under its own project, and author is
// unique fleet-wide, so a hit elsewhere can never belong to a different agent.
func (s *Store) ReviewingPR(home, author string) (project, pr string, err error) {
	if pr, err = s.For(home).ReviewingPR(author); err != nil || pr != "" {
		return home, pr, err
	}
	err = s.db.QueryRow(
		`SELECT project, pr FROM reviews WHERE author=? AND verdict='' ORDER BY id DESC LIMIT 1`,
		author).Scan(&project, &pr)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("reviewing pr for %s: %w", author, err)
	}
	return project, pr, nil
}

// RuledPRs is home's own list if it has any, else every project's — a pooled reviewer's verdicts
// are filed wherever it was sent, never its own project.
func (s *Store) RuledPRs(home, author string) ([]string, error) {
	local, err := s.For(home).RuledPRs(author)
	if err != nil || len(local) > 0 {
		return local, err
	}
	rows, err := s.db.Query(`SELECT pr FROM reviews WHERE author=? AND verdict<>'' ORDER BY id DESC`, author)
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
