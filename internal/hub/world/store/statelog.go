// package: hub/world/store / statelog
// type:    adapter (SQLite, hub-owned)
// job:     an agent's state log — the TRANSITIONS that are its history, and the PASS rows the
// machine narrates while looking at it, each trimmed against its own budget so the noisier can
// never cost the agent the quieter.
// limits:  the log. What a transition MEANS is the flow's, and the state rows themselves are
// workflow.go's.
package store

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
)

// StateReason is why an agent's stored state changed, or why its DERIVED status word did — a small
// closed set, not free text, so a transition is grep-able rather than decaying into "state changed".
type StateReason string

const (
	ReasonClaimed   StateReason = "claimed"   // started new work: a task, a subtask, a held container
	ReasonAdvanced  StateReason = "advanced"  // moved within held work: gating, resolving, the next subtask
	ReasonRejected  StateReason = "rejected"  // sent back for more work
	ReasonLanded    StateReason = "landed"    // merged or approved — the work is done
	ReasonFreed     StateReason = "freed"     // released back to resting: closed, scrapped, unassigned, discarded
	ReasonEscalated StateReason = "escalated" // stopped on a question only the user can answer
	ReasonStatus    StateReason = "status"    // the DERIVED status word changed (-> statuswatch.go), not a stored write
)

// Two things live in state_log and they are not the same thing. A TRANSITION is a change to what
// the hub stores about an agent — the history somebody reads to answer "what happened to it". A PASS
// row is the machine narrating one look (-> machine.Step): high volume, worth minutes, worthless
// after. They are trimmed against SEPARATE budgets, because one cap shared between them is always
// won by the noisier, and the noisier is always the less valuable — thrain's whole claim chain was
// pushed out by a prod re-running every two seconds, leaving three rows of history in five hundred.
const (
	stateLogCap = 500
	passLogCap  = 200
)

// transitionReasons is the closed set that counts as history. Built from the constants rather than
// written out again, so a new reason cannot be added in one place and forgotten here.
var transitionReasons = []StateReason{
	ReasonClaimed, ReasonAdvanced, ReasonRejected, ReasonLanded, ReasonFreed, ReasonEscalated, ReasonStatus,
}

// transitionList is those reasons as a SQL list. The values are Go constants with no quotes in them,
// so they inline safely and the trim needs no parameter fan-out.
var transitionList = func() string {
	quoted := make([]string, len(transitionReasons))
	for i, r := range transitionReasons {
		quoted[i] = "'" + string(r) + "'"
	}
	return "(" + strings.Join(quoted, ",") + ")"
}()

// LogState appends a state-log row and trims that agent's history back to its budget, oldest first.
func (p *ProjectStore) LogState(agent string, reason StateReason, detail string) error {
	return p.LogPass(agent, string(reason), "", detail)
}

// LogPass appends one step of a pass under the id that pass carries, so a decision, the action it
// started and the outcome read back as one story.
//
// A row identical to the one before it is not appended: a spell of the same thing happening says
// what one row says, and repeating it is how a history comes to hold nothing but the last five
// minutes of a loop. The earlier row stands, so the spell reads from when it STARTED.
func (p *ProjectStore) LogPass(agent, reason, pass, detail string) error {
	if same, err := p.repeatsLast(agent, reason, detail); err != nil || same {
		return err
	}
	if _, err := p.s.db.Exec(
		`INSERT INTO state_log (project, agent, ts, reason, pass, detail) VALUES (?,?,?,?,?,?)`,
		p.project, agent, time.Now().UTC().Format(time.RFC3339), reason, pass, detail); err != nil {
		return fmt.Errorf("log state for %s/%s: %w", p.project, agent, err)
	}
	return p.trimLog(agent, reason)
}

// repeatsLast reports this row saying exactly what the agent's newest one already says.
func (p *ProjectStore) repeatsLast(agent, reason, detail string) (bool, error) {
	var lastReason, lastDetail string
	err := p.s.db.QueryRow(
		`SELECT reason, detail FROM state_log WHERE project=? AND agent=? ORDER BY id DESC LIMIT 1`,
		p.project, agent).Scan(&lastReason, &lastDetail)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read last state-log row for %s/%s: %w", p.project, agent, err)
	}
	return lastReason == reason && lastDetail == detail, nil
}

// trimLog bounds the class the new row belongs to, leaving the other class untouched — which is the
// whole point: a burst of passes must not cost the agent its history.
func (p *ProjectStore) trimLog(agent, reason string) error {
	within, budget := "IN "+transitionList, stateLogCap
	if !slices.Contains(transitionReasons, StateReason(reason)) {
		within, budget = "NOT IN "+transitionList, passLogCap
	}
	q := `DELETE FROM state_log WHERE project=? AND agent=? AND reason ` + within + ` AND id NOT IN
		 (SELECT id FROM state_log WHERE project=? AND agent=? AND reason ` + within + ` ORDER BY id DESC LIMIT ?)`
	if _, err := p.s.db.Exec(q, p.project, agent, p.project, agent, budget); err != nil {
		return fmt.Errorf("trim state log for %s/%s: %w", p.project, agent, err)
	}
	return nil
}

// StateEvent is one row of the debug state log; it crosses the wire, so it is internal/api.StateEvent
// under the name every existing caller here already uses.
type StateEvent = api.StateEvent

// StateLog returns an agent's state-log rows, newest first, capped at limit (limit <= 0 means all —
// bounded anyway by stateLogCap's own trim on write).
func (p *ProjectStore) StateLog(agent string, limit int) ([]StateEvent, error) {
	q := `SELECT id, project, agent, ts, reason, pass, detail FROM state_log WHERE project=? AND agent=? ORDER BY id DESC`
	args := []any{p.project, agent}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := p.s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("state log for %s/%s: %w", p.project, agent, err)
	}
	defer rows.Close()
	var out []StateEvent
	for rows.Next() {
		var e StateEvent
		if err := rows.Scan(&e.ID, &e.Project, &e.Agent, &e.TS, &e.Reason, &e.Pass, &e.Detail); err != nil {
			return nil, fmt.Errorf("scan state log for %s/%s: %w", p.project, agent, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
