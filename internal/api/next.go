// package: api / next
// type:    logic (the assignment explanation, on the wire + one pure predicate)
// job:     what the hub would hand an agent next and why every other open task is not
// that — the answer to "why is nothing being assigned", carried to both
// front-ends so neither has to work it out for itself.
// limits:  the wire shape only; the reasoning is the workflow's (-> ExplainNext).
package api

// Claimability is why a task is or is not available to be handed out. Stated in the words a user
// would use about their own backlog, since the whole point is not having to read the queries.
type Claimability string

const (
	ClaimableTask    Claimability = "claimable"           // an open leaf a worker can be given
	ClaimablePackage Claimability = "claimable (package)" // a tree a worker takes whole
	AwaitingApproval Claimability = "awaiting your verdict"
	Rejected         Claimability = "rejected — the planner revises it"
	Unrated          Claimability = "unrated — no priority, no assignment"
	InsideAPackage   Claimability = "inside an open package, worked there"
	AlreadyHeld      Claimability = "held by an agent"
	// ChildrenGated: the package itself is ready, and every open task under it awaits a verdict.
	ChildrenGated Claimability = "its subtasks await your verdict"
	// Stranded is the catch-all: open, rated, ungated, unheld and offered by no pool. It should
	// never appear — if it does, some pool has a hole in it (-> td-8c0187).
	Stranded Claimability = "stranded — no pool offers it, which is a bug"
)

// TaskReason is one open task and where it stands.
type TaskReason struct {
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	Priority string       `json:"priority"`
	Why      Claimability `json:"why"`
	Note     string       `json:"note,omitempty"` // what to do about it, when there is something
}

// Claimable reports whether this task could be handed out now.
func (t TaskReason) Claimable() bool {
	return t.Why == ClaimableTask || t.Why == ClaimablePackage
}

// NextExplain is the whole answer: what an agent would get, and the standing of everything else.
type NextExplain struct {
	Agent string `json:"agent,omitempty"`
	// AgentNote is why this agent can take nothing regardless of the backlog (retired for a full
	// context, or already holding work). Empty when the agent is free to claim.
	AgentNote string `json:"agentNote,omitempty"`
	// Pick is what would be handed out, nil when nothing would be.
	Pick  *TaskReason  `json:"pick,omitempty"`
	Tasks []TaskReason `json:"tasks"`
}
