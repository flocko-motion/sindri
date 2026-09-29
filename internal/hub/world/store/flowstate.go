// package: hub/world/store / flowstate
// type:    adapter (SQLite, hub-owned)
// job:     where each machine has a subject that is not an agent — a merge intent and a queued run —
// and the one request a human makes of a merge intent. Its own column beside the status, because a
// state the machine cannot store is one it can never enter.
// limits:  the columns. An agent's own state lives with the rest of its row (-> workflow.go), and
// WHAT any of these names means is the flow's (-> hub/flow).
package store

import (
	"database/sql"
	"fmt"
	"time"
)

// PRState is where the machine has a merge intent, and when it got there. "open" alone cannot tell a
// PR nobody has read from one a reviewer holds, and both from one at the gate.
func (p *ProjectStore) PRState(id string) (state, since string, err error) {
	row := p.s.db.QueryRow(`SELECT state, state_since FROM prs WHERE project=? AND id=?`, p.project, id)
	if err := row.Scan(&state, &since); err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil
		}
		return "", "", fmt.Errorf("pr state %s: %w", id, err)
	}
	return state, since, nil
}

// SetPRState moves a merge intent, stamping only a real change so a state's age is its own.
func (p *ProjectStore) SetPRState(id, state string) error {
	_, err := p.s.db.Exec(
		`UPDATE prs SET state=?, state_since=CASE WHEN state=? THEN state_since ELSE ? END WHERE project=? AND id=?`,
		state, state, time.Now().UTC().Format(time.RFC3339), p.project, id)
	if err != nil {
		return fmt.Errorf("set pr state %s: %w", id, err)
	}
	return nil
}

// RunState is where the machine has a run, and when it got there. Its own column because a state the
// machine cannot store is one it can never enter — "dropping" has no status of its own.
func (p *ProjectStore) RunState(id string) (state, since string, err error) {
	row := p.s.db.QueryRow(`SELECT state, state_since FROM runs WHERE project=? AND id=?`, p.project, id)
	if err := row.Scan(&state, &since); err != nil {
		if err == sql.ErrNoRows {
			return "", "", nil
		}
		return "", "", fmt.Errorf("run state %s: %w", id, err)
	}
	return state, since, nil
}

// SetRunState moves a run, stamping only a real change so a state's age is its own.
func (p *ProjectStore) SetRunState(id, state string) error {
	_, err := p.s.db.Exec(
		`UPDATE runs SET state=?, state_since=CASE WHEN state=? THEN state_since ELSE ? END WHERE project=? AND id=?`,
		state, state, time.Now().UTC().Format(time.RFC3339), p.project, id)
	if err != nil {
		return fmt.Errorf("set run state %s: %w", id, err)
	}
	return nil
}

// SetMergeAsked records, or takes back, a human's request to merge this pull request. Kept off the
// PR row's own shape: it is an instruction to the hub rather than anything the board renders, and
// the state the request leads to is what a reader looks at (-> flow/pr's merging).
func (p *ProjectStore) SetMergeAsked(id string, asked bool) error {
	_, err := p.s.db.Exec(`UPDATE prs SET merge_asked=? WHERE project=? AND id=?`, asked, p.project, id)
	if err != nil {
		return fmt.Errorf("set merge asked %s: %w", id, err)
	}
	return nil
}

// MergeAsked reports whether a human has asked for this merge and it has not yet been carried out.
func (p *ProjectStore) MergeAsked(id string) (bool, error) {
	var asked bool
	err := p.s.db.QueryRow(`SELECT merge_asked FROM prs WHERE project=? AND id=?`, p.project, id).Scan(&asked)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("merge asked %s: %w", id, err)
	}
	return asked, nil
}
