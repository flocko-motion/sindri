// package: hub/store / submit answers
// type:    adapter (SQLite, hub-owned)
// job:     what an author answered before a submit was accepted, kept per COMMIT so editing the
// code retires the answers with the tree they described.
// limits:  rows only; which questions are asked, and what an answer is worth, are the workflow's.
package store

import (
	"fmt"
	"time"
)

// submitGateSchema keys on the sha, which is the whole reset mechanism: a change makes a new commit,
// the answers no longer match, and the questions come again. No invalidation logic of its own.
const submitGateSchema = `
CREATE TABLE IF NOT EXISTS submit_answers (
  project  TEXT NOT NULL,
  agent    TEXT NOT NULL,
  sha      TEXT NOT NULL,
  seq      INTEGER NOT NULL,
  question TEXT NOT NULL,
  answer   TEXT NOT NULL,
  at       TEXT NOT NULL,
  PRIMARY KEY (project, agent, sha, seq)
);
`

// SubmitAnswer is one question put to an author and what it said back.
type SubmitAnswer struct {
	Seq      int    `json:"seq"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	At       string `json:"at"`
}

// SubmitAnswers is what this agent has answered about this commit, in order asked.
func (p *ProjectStore) SubmitAnswers(agent, sha string) ([]SubmitAnswer, error) {
	rows, err := p.s.db.Query(
		`SELECT seq, question, answer, at FROM submit_answers WHERE project=? AND agent=? AND sha=? ORDER BY seq`,
		p.project, agent, sha)
	if err != nil {
		return nil, fmt.Errorf("submit answers for %s: %w", agent, err)
	}
	defer rows.Close()
	var out []SubmitAnswer
	for rows.Next() {
		var a SubmitAnswer
		if err := rows.Scan(&a.Seq, &a.Question, &a.Answer, &a.At); err != nil {
			return nil, fmt.Errorf("submit answers for %s: %w", agent, err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AddSubmitAnswer records one. Re-answering the same question replaces it, so a repeated call
// corrects rather than duplicating.
func (p *ProjectStore) AddSubmitAnswer(agent, sha string, seq int, question, answer string) error {
	_, err := p.s.db.Exec(`
		INSERT INTO submit_answers (project, agent, sha, seq, question, answer, at) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(project, agent, sha, seq) DO UPDATE SET question=excluded.question,
		  answer=excluded.answer, at=excluded.at`,
		p.project, agent, sha, seq, question, answer, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("record submit answer %d for %s: %w", seq, agent, err)
	}
	return nil
}
