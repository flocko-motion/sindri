// package: api / pr
// type:    logic (wire types + a pure predicate)
// job:     a PR (merge-intent) as it crosses the wire, its detail view, the review
// items attached to it, and the open-ness rule every PR list applies.
// limits:  data and a pure predicate only; no persistence, no rendering.
package api

import "fmt"

// PR is a merge-intent; it carries its project so the global board can tag the repo.
type PR struct {
	Project   string `json:"project"`
	ID        string `json:"id"`
	Task      string `json:"task"`
	Agent     string `json:"agent"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Status    string `json:"status"`
	Feedback  string `json:"feedback"`
	CreatedAt string `json:"created_at"`
	// Kind: a final PR's merge closes the task, an interim one keeps it open and puts
	// the worker straight back on it. "" is read as "final".
	Kind string `json:"kind"`
	// Reviewer is the agent holding an open review of this PR, "" when nobody is. Derived from the
	// reviews table by the hub rather than stored on the row, and carried here because whether a PR
	// is being looked at is the hub's answer to give — a front-end that worked it out for itself
	// would be deciding, not rendering.
	Reviewer string `json:"reviewer,omitempty"`
	// Approvals is how many approval verdicts this PR has accumulated — reviewer, human and
	// planner badges alike. Derived (-> ApprovalCount), same reasoning as Reviewer: a list row has
	// no Reviews array of its own, so the hub counts once and carries the number.
	Approvals int `json:"approvals,omitempty"`
}

// Review is one review item attached to a PR.
type Review struct {
	ID          int64  `json:"id"`
	PR          string `json:"pr"`
	Requirement string `json:"requirement"`
	Author      string `json:"author"`
	Verdict     string `json:"verdict"`
	Result      string `json:"result"`
	CreatedAt   string `json:"created_at"`
	ReviewAt    string `json:"review_at"`
	VerdictAt   string `json:"verdict_at"`
	// Advisory marks a planner's optional badge: recorded and visible like any other verdict, but
	// it never moves pr.Status to "approved" — a planner is not an independent reviewer, so its
	// opinion is a second perspective beside the real one, never a substitute for it.
	Advisory bool `json:"advisory,omitempty"`
}

// PRDetail is a merge-intent plus its linked task and diff (for `pr info`).
type PRDetail struct {
	PR      PR       `json:"pr"`
	Task    Task     `json:"task"`
	Diff    string   `json:"diff"`
	Reviews []Review `json:"reviews"`
	Lint    string   `json:"lint"`    // latest stored lint output ("" = never run)
	LintAt  string   `json:"lint_at"` // when it was run
	History []Event  `json:"history"` // lifecycle log (oldest-first)
}

// PROpen reports whether a PR is still open — in neither terminal state (merged or
// scrapped). Exported because a UI that narrows the board to one repo has to apply the
// SAME open-ness rule to its subset that OpenPRCount applies to the whole fleet; if it
// reimplemented the rule, a tab badge could disagree with the list beneath it.
func PROpen(p PR) bool { return p.Status != "merged" && p.Status != "scrapped" }

// PRApprovable reports whether a fresh approval may be recorded: the PR is still being decided,
// not already rejected (which only a renewed submission clears), merging, or settled. Approvals
// accumulate, so unlike the old single-verdict model this stays true past the first approval.
func PRApprovable(p PR) bool { return p.Status == "open" || p.Status == "approved" }

// ApprovalCount is how many of revs are approvals ("pass" verdicts) — reviewer, human, and
// planner badges alike, since the count a person sees is not the same thing as what gates the
// merge (PRApprovable and the "approved" status it protects count a planner's badge for nothing).
func ApprovalCount(revs []Review) int {
	n := 0
	for _, r := range revs {
		if r.Verdict == "pass" {
			n++
		}
	}
	return n
}

// StatusLabel is a PR's status as shown to a person: "approved(2)" once it carries that many
// approval badges, so the count reads as part of the status rather than a separate column that
// could drift from it.
func StatusLabel(status string, approvals int) string {
	if status == "approved" && approvals > 0 {
		return fmt.Sprintf("approved(%d)", approvals)
	}
	return status
}
