// package: hub/world/store / submit answers
// type:    adapter (SQLite, hub-owned)
// job:     the interview an author sits through before a submit is taken — one open one per agent,
// every question written down when it opens and each answer filled in as it arrives, finished when
// the submit it belongs to is taken.
// limits:  rows only; which questions are asked, and what an answer is worth, are hub/flow/pr's.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// submitGateSchema records the tree the interview OPENED at, in the column that once held a commit.
// A name for the working tree rather than a commit (-> git.TreeFingerprint), because nothing is
// committed until the questions are through: the tree the answers describe has to be the tree that
// goes up, and an author who edits mid-interview is answering about a tree that no longer exists.
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

// OpenSubmitAnswers is the agent's unfinished interview: the tree it opened at and every question
// written down for it, in order asked, each with the answer it has so far. An empty answer on a row
// is a question still standing. An empty key means there is no interview open.
func (p *ProjectStore) OpenSubmitAnswers(agent string) (string, []SubmitAnswer, error) {
	var tree string
	err := p.s.db.QueryRow(
		`SELECT sha FROM submit_answers WHERE project=? AND agent=? AND done=0 ORDER BY at DESC LIMIT 1`,
		p.project, agent).Scan(&tree)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("open interview for %s: %w", agent, err)
	}
	rows, err := p.s.db.Query(
		`SELECT seq, question, answer, at FROM submit_answers
		 WHERE project=? AND agent=? AND sha=? AND done=0 ORDER BY seq`, p.project, agent, tree)
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
	return tree, out, rows.Err()
}

// FinishSubmitAnswers closes a questionnaire once its submit has been taken, so the next attempt
// opens a fresh one. Marked rather than deleted: it is the record of what the author was asked.
func (p *ProjectStore) FinishSubmitAnswers(agent, tree string) error {
	_, err := p.s.db.Exec(`UPDATE submit_answers SET done=1 WHERE project=? AND agent=? AND sha=?`,
		p.project, agent, tree)
	if err != nil {
		return fmt.Errorf("finish interview %s for %s: %w", tree, agent, err)
	}
	return nil
}

// OpenSubmitInterview writes a whole interview down at once: the summary in row 0, then one row per
// question with no answer yet. Written when it OPENS rather than question by question, so what the
// author is being asked survives a hub that dies mid-exchange and the interview resumes on the same
// questions rather than drawing fresh ones.
//
// Any interview still open for this agent is finished first: one at a time, so "the open one" is
// never a choice between two.
func (p *ProjectStore) OpenSubmitInterview(agent, tree, summary string, questions []string) error {
	if prev, _, err := p.OpenSubmitAnswers(agent); err != nil {
		return err
	} else if prev != "" && prev != tree {
		if err := p.FinishSubmitAnswers(agent, prev); err != nil {
			return err
		}
	}
	if err := p.AddSubmitAnswer(agent, tree, 0, SubmitSummaryRow, summary); err != nil {
		return err
	}
	for i, q := range questions {
		if err := p.AddSubmitAnswer(agent, tree, i+1, q, ""); err != nil {
			return err
		}
	}
	return nil
}

// SubmitSummaryRow marks row 0, which holds the submit summary rather than an answer.
const SubmitSummaryRow = "(the submit summary)"

// AddSubmitAnswer records one. Re-answering the same question replaces it, so a repeated call
// corrects rather than duplicating.
func (p *ProjectStore) AddSubmitAnswer(agent, tree string, seq int, question, answer string) error {
	_, err := p.s.db.Exec(`
		INSERT INTO submit_answers (project, agent, sha, seq, question, answer, at) VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(project, agent, sha, seq) DO UPDATE SET question=excluded.question,
		  answer=excluded.answer, at=excluded.at`,
		p.project, agent, tree, seq, question, answer, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("record submit answer %d for %s: %w", seq, agent, err)
	}
	return nil
}
