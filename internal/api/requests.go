// package: api / requests
// type:    logic (wire types)
// job:     the request bodies the hub's HTTP endpoints decode — one type per shape
// they share, named for what they carry rather than any one route.
// limits:  data only; decoding and dispatch stay in internal/hub.
package api

// AgentReq is the body for POST /agents.
type AgentReq struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Memory string `json:"memory"` // optional per-agent RAM limit (e.g. "4g"); "" = hub default
}

// TellReq is the body for POST /tell.
type TellReq struct {
	Name   string `json:"name"`
	Msg    string `json:"msg"`
	Source string `json:"source"`
}

// PlanReq is the body for POST /agent/plan. Task names an existing task to work up, whose title and
// body become the brief; Goal is free text, and may accompany a task to add something it lacks.
type PlanReq struct {
	Name string `json:"name"`
	Goal string `json:"goal"`
	Task string `json:"task"`
}

// ChatSayReq is the body for POST /chat/say — the user posting to the room.
type ChatSayReq struct {
	Msg string `json:"msg"`
}

// NameReq is the body for operations addressing one agent (POST /launch) or PR
// (POST /merge). Shell and Debug apply to /launch only.
type NameReq struct {
	Name   string `json:"name"`
	Shell  bool   `json:"shell"`
	Debug  bool   `json:"debug"`  // stream the hub's liveness-probe detail during the launch wait
	Memory string `json:"memory"` // set an agent's RAM limit (POST /agent/memory)
}

// RepoReq targets a registered repo by its tag (POST /repo/forget, /repo/color).
type RepoReq struct {
	Tag   string `json:"tag"`
	Color int    `json:"color"` // colour choice for /repo/color
}

// PriorityReq is the body for POST /priority.
type PriorityReq struct {
	ID       string `json:"id"`
	Priority string `json:"priority"`
}

// RejectReq is the body for POST /pr/reject.
type RejectReq struct {
	ID       string `json:"id"`
	Feedback string `json:"feedback"`
}

// ApproveTaskReq is the body for POST /task/approve: the task, and whether the verdict
// carries down to the tasks below it that still await one.
type ApproveTaskReq struct {
	ID      string `json:"id"`
	Subtree bool   `json:"subtree"`
}

// ScrapTaskReq is the body for POST /task/delete: the task, and how far the discard
// reaches — down its subtree, and over the open PRs of what it scraps.
type ScrapTaskReq struct {
	ID      string `json:"id"`
	Subtree bool   `json:"subtree"`
	PRs     bool   `json:"prs"`
}

// TaskReq is the body for POST /tasks (create) and POST /task/edit (ID set).
type TaskReq struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Priority    string   `json:"priority"`
	Parent      string   `json:"parent"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
}

// Spec is req's payload as a TaskSpec, for CreateTask/EditTask.
func (r TaskReq) Spec() TaskSpec {
	return TaskSpec{Title: r.Title, Type: r.Type, Priority: r.Priority, Parent: r.Parent, Description: r.Description, Labels: r.Labels}
}
