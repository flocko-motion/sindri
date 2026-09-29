// package: hub/world/store / submit intent
// type:    adapter (SQLite, hub-owned)
// job:     the one submit an author has asked for and the hub has not yet taken — the tree it was
// asked on and the summary it was asked with. A REQUEST, recorded so it survives a hub that dies
// between the asking and the taking.
// limits:  the row. What is asked of the author before it is taken is hub/flow/pr's, and when the
// request is carried out is the author's own map's (-> worker/interviewing).
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// submitIntentSchema holds one request per agent: an author submits what it holds, so a second
// request replaces the first rather than queueing behind it.
const submitIntentSchema = `
CREATE TABLE IF NOT EXISTS submit_intents (
  project TEXT NOT NULL,
  agent   TEXT NOT NULL,
  tree    TEXT NOT NULL,
  summary TEXT NOT NULL,
  at      TEXT NOT NULL,
  PRIMARY KEY (project, agent)
);
`

// SubmitIntent is a submit asked for and not yet taken.
type SubmitIntent struct {
	// Tree names the working tree the submit was asked on (-> git.TreeFingerprint). The answers
	// describe that tree, so a tree that no longer matches this is a submit that has to start again.
	Tree    string `json:"tree"`
	Summary string `json:"summary"`
	At      string `json:"at"`
}

// AskSubmit records an author's request to submit what it holds, on the tree it holds it in.
func (p *ProjectStore) AskSubmit(agent, tree, summary string) error {
	_, err := p.s.db.Exec(`
		INSERT INTO submit_intents (project, agent, tree, summary, at) VALUES (?,?,?,?,?)
		ON CONFLICT(project, agent) DO UPDATE SET tree=excluded.tree, summary=excluded.summary,
		  at=excluded.at`,
		p.project, agent, tree, summary, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("record %s's submit: %w", agent, err)
	}
	return nil
}

// SubmitAsked is the request standing for this agent, ok false when there is none.
func (p *ProjectStore) SubmitAsked(agent string) (in SubmitIntent, ok bool, err error) {
	row := p.s.db.QueryRow(`SELECT tree, summary, at FROM submit_intents WHERE project=? AND agent=?`,
		p.project, agent)
	if err := row.Scan(&in.Tree, &in.Summary, &in.At); errors.Is(err, sql.ErrNoRows) {
		return SubmitIntent{}, false, nil
	} else if err != nil {
		return SubmitIntent{}, false, fmt.Errorf("submit asked by %s: %w", agent, err)
	}
	return in, true, nil
}

// AnswerSubmitRequest takes the request back, for whoever carried it out or abandoned it. Left
// standing it would bring the author back into the interview on every beat, for ever.
func (p *ProjectStore) AnswerSubmitRequest(agent string) error {
	_, err := p.s.db.Exec(`DELETE FROM submit_intents WHERE project=? AND agent=?`, p.project, agent)
	if err != nil {
		return fmt.Errorf("answer %s's submit request: %w", agent, err)
	}
	return nil
}
