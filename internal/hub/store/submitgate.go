// package: hub/store / submit answers
// type:    adapter (SQLite, hub-owned)
// job:     what an author answered before a submit was accepted, kept per ATTEMPT — one open
// questionnaire per agent, finished when the submit it belongs to is taken.
// limits:  rows only; which questions are asked, and what an answer is worth, are the workflow's.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// submitGateSchema records the sha the questionnaire OPENED at, which fixes the questions for the
// whole attempt. Keying on the tree instead made answering reset the exercise: the questions send an
// author to the code, and a question like "which test fails if you revert that?" is often
// unanswerable without writing one — which changed the tree, which asked everything again.
const submitGateSchema = `
CREATE TABLE IF NOT EXISTS submit_answers (
  project  TEXT NOT NULL,
  agent    TEXT NOT NULL,
  sha      TEXT NOT NULL,
  seq      INTEGER NOT NULL,
  question TEXT NOT NULL,
  answer   TEXT NOT NULL,
  at       TEXT NOT NULL,
  done     INTEGER NOT NULL DEFAULT 0,
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

// OpenSubmitAnswers is the agent's unfinished questionnaire: the sha it opened at and what has been
// answered so far, in order asked. An empty sha means there is none open.
func (p *ProjectStore) OpenSubmitAnswers(agent string) (string, []SubmitAnswer, error) {
	var sha string
	err := p.s.db.QueryRow(
		`SELECT sha FROM submit_answers WHERE project=? AND agent=? AND done=0 ORDER BY at DESC LIMIT 1`,
		p.project, agent).Scan(&sha)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("open questionnaire for %s: %w", agent, err)
	}
	rows, err := p.s.db.Query(
		`SELECT seq, question, answer, at FROM submit_answers
		 WHERE project=? AND agent=? AND sha=? AND done=0 ORDER BY seq`, p.project, agent, sha)
	if err != nil {
		return "", nil, fmt.Errorf("submit answers for %s: %w", agent, err)
	}
	defer rows.Close()
	var out []SubmitAnswer
	for rows.Next() {
		var a SubmitAnswer
		if err := rows.Scan(&a.Seq, &a.Question, &a.Answer, &a.At); err != nil {
			return "", nil, fmt.Errorf("submit answers for %s: %w", agent, err)
		}
		out = append(out, a)
	}
	return sha, out, rows.Err()
}

// FinishSubmitAnswers closes a questionnaire once its submit has been taken, so the next attempt
// opens a fresh one. Marked rather than deleted: it is the record of what the author was asked.
func (p *ProjectStore) FinishSubmitAnswers(agent, sha string) error {
	_, err := p.s.db.Exec(`UPDATE submit_answers SET done=1 WHERE project=? AND agent=? AND sha=?`,
		p.project, agent, sha)
	if err != nil {
		return fmt.Errorf("finish questionnaire %s for %s: %w", sha, agent, err)
	}
	return nil
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
