// package: api / pr
// type:    logic (wire types + a pure predicate)
// job:     a PR (merge-intent) as it crosses the wire, its detail view, the review
// items attached to it, and the open-ness rule every PR list applies.
// limits:  data and a pure predicate only; no persistence, no rendering.
package api

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
