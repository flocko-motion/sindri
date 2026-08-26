// package: hub/store / reviews
// type:    adapter (SQLite, hub-owned)
// job:     a PR's reviews — who was asked, who holds one, what each ruled — and the queries
// the pool and the board read them back through.
// limits:  rows only; who may review what, and what a verdict means, are the workflow's.
package store

import (
	"database/sql"
	"fmt"
	"time"
)

const reviewCols = `SELECT id,pr,requirement,author,verdict,result,created_at,review_at,verdict_at,advisory FROM reviews`

// AddReview attaches a requirement to a PR in this project, unassigned. Returns id.
func (p *ProjectStore) AddReview(pr, requirement string) (int64, error) {
	res, err := p.s.db.Exec(`INSERT INTO reviews (project, pr, requirement, created_at) VALUES (?,?,?,?)`,
		p.project, pr, requirement, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("add review: %w", err)
	}
	return res.LastInsertId()
}

// AddVerdict records a completed review directly, author and verdict together — for a verdict
// that never went through the assign flow: a human's approve/reject, or a planner's advisory
// badge. Returns the new row's id.
func (p *ProjectStore) AddVerdict(pr, requirement, author, verdict, result string, advisory bool) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := p.s.db.Exec(
		`INSERT INTO reviews (project, pr, requirement, author, review_at, verdict, result, verdict_at, advisory, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.project, pr, requirement, author, now, verdict, result, now, advisory, now)
	if err != nil {
		return 0, fmt.Errorf("add verdict: %w", err)
	}
	return res.LastInsertId()
}

// ApprovalCounts maps each PR in this project to how many approvals it has accumulated —
// reviewer, human and planner badges alike, since this is what a person sees, not what gates
// the merge (-> api.PRApprovable, which reads pr.Status instead).
func (p *ProjectStore) ApprovalCounts() (map[string]int, error) {
	rows, err := p.s.db.Query(
		`SELECT pr, COUNT(*) FROM reviews WHERE project=? AND verdict='pass' GROUP BY pr`, p.project)
	if err != nil {
		return nil, fmt.Errorf("approval counts: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var pr string
		var n int
		if err := rows.Scan(&pr, &n); err != nil {
			return nil, err
		}
		out[pr] = n
	}
	return out, rows.Err()
}

// AssignReview marks a review as picked up by an author (in progress).
func (p *ProjectStore) AssignReview(id int64, author string) error {
	_, err := p.s.db.Exec(`UPDATE reviews SET author=?, review_at=? WHERE id=? AND project=?`,
		author, time.Now().UTC().Format(time.RFC3339), id, p.project)
	if err != nil {
		return fmt.Errorf("assign review %d: %w", id, err)
	}
	return nil
}

// UnclaimedReview returns the oldest review nobody is doing, for a PR that is still open — what a
// reviewer picks up when it finishes one and is free again, and what covers a review requested
// while no reviewer was running. (0, "", false) when there is none.
func (p *ProjectStore) UnclaimedReview(id *int64, pr *string) (bool, error) {
	err := p.s.db.QueryRow(`
		SELECT r.id, r.pr FROM reviews r JOIN prs pp ON pp.project=r.project AND pp.id=r.pr
		WHERE r.project=? AND r.author='' AND r.verdict='' AND pp.status='open'
		ORDER BY r.id LIMIT 1`, p.project).Scan(id, pr)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("unclaimed review: %w", err)
	}
	return true, nil
}

// ActiveReviewers maps each PR in this project to the agent holding an open review of it. One query
// for the whole project, since every PR list wants it and a lookup per row would be paid per render.
func (p *ProjectStore) ActiveReviewers() (map[string]string, error) {
	rows, err := p.s.db.Query(
		`SELECT pr, author FROM reviews WHERE project=? AND verdict='' AND author!='' ORDER BY id`,
		p.project)
	if err != nil {
		return nil, fmt.Errorf("active reviewers: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var pr, author string
		if err := rows.Scan(&pr, &author); err != nil {
			return nil, err
		}
		out[pr] = author
	}
	return out, rows.Err()
}

// AmendReview replaces an open review's requirement, for a second instruction arriving while the
// first is still being carried out. The review is the same one: the reviewer keeps the branch it
// has checked out and is simply told more.
func (p *ProjectStore) AmendReview(id int64, requirement string) error {
	_, err := p.s.db.Exec(`UPDATE reviews SET requirement=? WHERE id=? AND project=?`,
		requirement, id, p.project)
	if err != nil {
		return fmt.Errorf("amend review %d: %w", id, err)
	}
	return nil
}

// CloseReviews ends every open review of a PR without a verdict — what a merge or a scrap does to a
// review that has been overtaken: the thing it was about is settled, so nobody should still hold it.
func (p *ProjectStore) CloseReviews(pr, why string) error {
	_, err := p.s.db.Exec(
		`UPDATE reviews SET verdict='moot', result=? WHERE project=? AND pr=? AND verdict=''`,
		why, p.project, pr)
	if err != nil {
		return fmt.Errorf("close reviews of %s: %w", pr, err)
	}
	return nil
}

// LiveReviewPRs is the set of PR ids in this project carrying at least one unverdicted review row
// — held or unclaimed. One query for the whole project (-> ActiveReviewers, ApprovalCounts), since
// the review-row invariant sweeps every open PR and a query per PR would be paid per one of them.
func (p *ProjectStore) LiveReviewPRs() (map[string]bool, error) {
	rows, err := p.s.db.Query(`SELECT DISTINCT pr FROM reviews WHERE project=? AND verdict=''`, p.project)
	if err != nil {
		return nil, fmt.Errorf("live review prs: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var pr string
		if err := rows.Scan(&pr); err != nil {
			return nil, err
		}
		out[pr] = true
	}
	return out, rows.Err()
}

// RecordVerdict completes a review with a verdict and the reviewer's findings.
func (p *ProjectStore) RecordVerdict(id int64, verdict, result string) error {
	_, err := p.s.db.Exec(`UPDATE reviews SET verdict=?, result=?, verdict_at=? WHERE id=? AND project=?`,
		verdict, result, time.Now().UTC().Format(time.RFC3339), id, p.project)
	if err != nil {
		return fmt.Errorf("record verdict %d: %w", id, err)
	}
	return nil
}

// Reviews lists a PR's review items in this project, oldest first.
func (p *ProjectStore) Reviews(pr string) ([]Review, error) {
	rows, err := p.s.db.Query(reviewCols+` WHERE project=? AND pr=? ORDER BY id`, p.project, pr)
	if err != nil {
		return nil, fmt.Errorf("reviews %s: %w", pr, err)
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.PR, &r.Requirement, &r.Author, &r.Verdict,
			&r.Result, &r.CreatedAt, &r.ReviewAt, &r.VerdictAt, &r.Advisory); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RuledPRs is every PR author has recorded a verdict on, newest first — what a reviewer may still
// comment on, its HELD review having ended the moment that verdict landed (-> ReviewingPR).
func (p *ProjectStore) RuledPRs(author string) ([]string, error) {
	rows, err := p.s.db.Query(
		`SELECT pr FROM reviews WHERE project=? AND author=? AND verdict<>'' ORDER BY id DESC`,
		p.project, author)
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

// ReviewingPR is the newest verdict-less review assigned to author, "" if none. The board
// needs it because a reviewer authors no PR, leaving its AgentView.PR empty.
func (p *ProjectStore) ReviewingPR(author string) (string, error) {
	var pr string
	err := p.s.db.QueryRow(
		`SELECT pr FROM reviews WHERE project=? AND author=? AND verdict='' ORDER BY id DESC LIMIT 1`,
		p.project, author).Scan(&pr)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reviewing pr for %s: %w", author, err)
	}
	return pr, nil
}

// SubmitCounts is how many times each PR in this project has been put up — its created event plus
// every resubmission. One query for the project rather than one per PR: the board asks for all of
// them at once, and a query per row is what the reviewer fill already avoids.
func (p *ProjectStore) SubmitCounts() (map[string]int, error) {
	rows, err := p.s.db.Query(
		`SELECT pr, COUNT(*) FROM pr_events WHERE project=? AND type IN ('created','resubmitted') GROUP BY pr`,
		p.project)
	if err != nil {
		return nil, fmt.Errorf("submit counts: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var pr string
		var n int
		if err := rows.Scan(&pr, &n); err != nil {
			return nil, fmt.Errorf("submit counts: %w", err)
		}
		out[pr] = n
	}
	return out, rows.Err()
}
