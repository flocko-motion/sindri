// package: hub/store / gate
// type:    adapter (SQLite, hub-owned)
// job:     what the quality gate said — the latest result for a PR, and every commit's verdict with
// the verify command that produced it — plus who is waiting to be told a run has landed.
// limits:  rows only. Which verdict may be reused, and who gets told, are the workflow's
// (-> hub/workflow/gate.go). The schema lives in workflow.go with its siblings.
package store

import (
	"fmt"
	"time"
)

// SetPRLint stores a PR's latest gate output, against the commit it describes.
func (p *ProjectStore) SetPRLint(prID, sha, output string) error {
	_, err := p.s.db.Exec(`INSERT INTO pr_lint (project, pr, output, ran_at, sha) VALUES (?,?,?,?,?)
		ON CONFLICT(project, pr) DO UPDATE SET output=excluded.output, ran_at=excluded.ran_at, sha=excluded.sha`,
		p.project, prID, output, time.Now().UTC().Format(time.RFC3339), sha)
	if err != nil {
		return fmt.Errorf("set pr lint %s: %w", prID, err)
	}
	return nil
}

// GetPRLint returns that output, the commit it was run against, and when.
func (p *ProjectStore) GetPRLint(prID string) (output, sha, ranAt string) {
	_ = p.s.db.QueryRow(`SELECT output, sha, ran_at FROM pr_lint WHERE project=? AND pr=?`, p.project, prID).
		Scan(&output, &sha, &ranAt)
	return output, sha, ranAt
}

// SetGateResult records what the gate said about one commit; only a pass is ever reused, and an
// empty sha (a tree the hub does not own) is not a commit and records nothing.
func (p *ProjectStore) SetGateResult(sha string, passed bool, verify, output string) error {
	if sha == "" {
		return nil
	}
	_, err := p.s.db.Exec(`INSERT INTO gate_result (project, sha, passed, verify, output, ran_at) VALUES (?,?,?,?,?,?)
		ON CONFLICT(project, sha) DO UPDATE SET passed=excluded.passed, verify=excluded.verify,
			output=excluded.output, ran_at=excluded.ran_at`,
		p.project, sha, passed, verify, output, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("set gate result for %s: %w", sha, err)
	}
	return nil
}

// GateVerdict reports what the gate said about this commit under the same verify command — pass or
// fail. For READING a result back: a reader that must re-run the gate to see why it failed pays for
// one every time it looks.
func (p *ProjectStore) GateVerdict(sha, verify string) (output string, passed, ok bool) {
	if sha == "" {
		return "", false, false
	}
	var storedVerify string
	err := p.s.db.QueryRow(`SELECT passed, verify, output FROM gate_result WHERE project=? AND sha=?`,
		p.project, sha).Scan(&passed, &storedVerify, &output)
	if err != nil || storedVerify != verify {
		return "", false, false
	}
	return output, passed, true
}

// GatePassed is the same read narrowed to a PASS — what a DECISION may stand on. A failure can come
// from outside the tree (a flaky test), and one that decided anything would need a human to unpin.
func (p *ProjectStore) GatePassed(sha, verify string) (output string, ok bool) {
	out, passed, ok := p.GateVerdict(sha, verify)
	if !ok || !passed {
		return "", false
	}
	return out, true
}

// AddRunWaiter records an agent to tell when this run lands. Idempotent: asking twice is one ask.
func (p *ProjectStore) AddRunWaiter(runID, agent string) error {
	_, err := p.s.db.Exec(`INSERT OR IGNORE INTO run_waiters (project, run, agent) VALUES (?,?,?)`,
		p.project, runID, agent)
	if err != nil {
		return fmt.Errorf("add waiter %s on run %s: %w", agent, runID, err)
	}
	return nil
}

// RunWaiters lists the agents waiting on a run. Kept after it lands, as the run row itself is: the
// record of who asked is worth the three short strings it costs.
func (p *ProjectStore) RunWaiters(runID string) ([]string, error) {
	rows, err := p.s.db.Query(`SELECT agent FROM run_waiters WHERE project=? AND run=? ORDER BY agent`,
		p.project, runID)
	if err != nil {
		return nil, fmt.Errorf("read waiters on run %s: %w", runID, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var agent string
		if err := rows.Scan(&agent); err != nil {
			return nil, err
		}
		out = append(out, agent)
	}
	return out, rows.Err()
}

// AgentWaitingOnRun reports whether agent's next move depends on a run the fleet's own queue is
// holding: one it queued itself (a self-check, a `sindri run`), or one it asked to be told about
// (-> AddRunWaiter, a reviewer's `sindri lint <pr>`). Either way the hub, not the agent, decides
// when it moves next (-> workflow.Stalled).
func (p *ProjectStore) AgentWaitingOnRun(agent string) (bool, error) {
	var waiting bool
	err := p.s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM runs WHERE project=? AND agent=? AND status IN ('queued','running'))
			OR EXISTS(SELECT 1 FROM run_waiters w JOIN runs r ON r.project=w.project AND r.id=w.run
				WHERE w.project=? AND w.agent=? AND r.status IN ('queued','running'))`,
		p.project, agent, p.project, agent).Scan(&waiting)
	if err != nil {
		return false, fmt.Errorf("check %s waiting on a run: %w", agent, err)
	}
	return waiting, nil
}
