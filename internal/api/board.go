// package: api / board
// type:    logic (the whole-board wire type + its badge counts)
// job:     the board every UI renders (BoardState), the views it carries, and its
// pure count methods — these make it satisfy hub/commands' Board interface
// without either side importing the other.
// limits:  data and pure counts only; assembling a BoardState is the hub's.
package api

// AgentView is an agent as the UIs see it. Status folds runtime and workflow into one word: the
// observed phase (idle, working, submitted, …), else launching/stopping/down/"unknown".
type AgentView struct {
	Project string `json:"project"`
	Repo    string `json:"repo"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Status  string `json:"status"`
	Task    string `json:"task"`
	// Feature is the parent task whose subtasks it is working, if any (gates the agent's verbs).
	Feature   string `json:"feature,omitempty"`
	Branch    string `json:"branch"`
	PR        string `json:"pr"`
	Workspace string `json:"workspace"` // the agent's git worktree path (repo-relative)
	Clients   int    `json:"clients"`   // humans attached to its tmux session (dial-ins)
	Container string `json:"container"` // podman container name (project-resolved, so cross-repo callers target the right pod)
	Memory    string `json:"memory"`    // configured RAM limit ("" = hub default)
	Runtime   string `json:"runtime"`   // Claude's live runtime: "working"|"blocked"|"idle"|"" (folded into Status; kept raw for the herdr projection)
	// ContextTokens is the agent's live session context size and ContextWindow the window it fills,
	// both read off its transcript (0 = not measured). Past workflow.ContextFullFraction of that
	// window an agent is retired from assignment until a human clears it; Status reads "full" only
	// where that explains an agent holding nothing, since elsewhere the word it would replace is the
	// one the column exists for. The window is per agent because it is the model's: one number for
	// the fleet retired 1M agents at 17%.
	ContextTokens int `json:"contextTokens"`
	ContextWindow int `json:"contextWindow"`
}

// RepoDocState is a repo's architecture-doc situation: the path in effect, and Advice ("" when fine).
type RepoDocState struct {
	Doc      string `json:"doc"`      // the path in effect: configured, else the default
	Set      bool   `json:"set"`      // the project named it (vs falling back to the default)
	Readable bool   `json:"readable"` // a doc exists at Doc
	Advice   string `json:"advice"`   // "" when nothing to say
}

// BoardState is the whole board: Agents and PRs global, Tasks only the selected project's.
type BoardState struct {
	Agents   []AgentView             `json:"agents"`
	Tasks    []Task                  `json:"tasks"`
	PRs      []PR                    `json:"prs"`
	Projects []Project               `json:"projects"`
	Orphans  []string                `json:"orphans"`   // pods with no roster entry (D14)
	Chat     ChatView                `json:"chat"`      // the user's chatroom: members + transcript
	RepoDocs map[string]RepoDocState `json:"repo_docs"` // per repo tag: its architecture doc + any gap
	// SpecCLIMissing: an openspec/ folder exists but the hub found no openspec CLI on its PATH.
	SpecCLIMissing bool `json:"spec_cli_missing"`
	// StartedAt is when this hub process came up (RFC3339), so `hub status` reads uptime from the board.
	StartedAt string `json:"started_at"`
	// DefaultMemory is the RAM an agent gets with none configured — the runtime's own current default.
	DefaultMemory string `json:"defaultMemory"`
}

// AgentStatsView is one agent's resource snapshot; Err is set, not swallowed into a misleading zero.
type AgentStatsView struct {
	Name          string `json:"name"`
	Repo          string `json:"repo"`
	MemUsageBytes int64  `json:"memUsageBytes"`
	MemLimitBytes int64  `json:"memLimitBytes"`
	Err           string `json:"err,omitempty"`
}

// StatsReport is the `agent stats` payload; Engine is included so the numbers read in context.
type StatsReport struct {
	Engine string           `json:"engine"`
	Agents []AgentStatsView `json:"agents"`
}

// StatusUnknown is an agent not yet observed — absence of a claim, not the claim "down" makes.
const StatusUnknown = "unknown"

// AgentNotUp reports whether a status rules out acting on a live pod — the one place these words
// are enumerated, so nothing else defaults an unknown status to "running".
func AgentNotUp(status string) bool {
	switch status {
	case "", "down", StatusUnknown, "launching", "stopping":
		return true
	}
	return false
}

// AgentNeedsLaunch reports whether an agent has no pod and none on the way — narrower than
// AgentNotUp, which also covers one already in flight.
func AgentNeedsLaunch(status string) bool {
	return status == "down" || status == StatusUnknown
}

// These satisfy commands.Board — the dashboard's badge counts.

// OpenTaskCount is the number of not-done tasks in the selected project.
func (b BoardState) OpenTaskCount() int { return countTasks(b.Tasks, Open) }

// AgentCount is the whole roster size (down agents are still agents).
func (b BoardState) AgentCount() int { return len(b.Agents) }

// OpenPRCount is open PRs across the fleet (neither merged nor scrapped), matching the PRs tab default.
func (b BoardState) OpenPRCount() int { return countPRs(b.PRs, PROpen) }

// RepoCount is the number of repos the hub tracks.
func (b BoardState) RepoCount() int { return len(b.Projects) }

// ChatMemberCount is the number of agents in the user's chatroom.
func (b BoardState) ChatMemberCount() int { return len(b.Chat.Members) }

func countTasks(ts []Task, pred func(Task) bool) (n int) {
	for _, t := range ts {
		if pred(t) {
			n++
		}
	}
	return
}

func countPRs(ps []PR, pred func(PR) bool) (n int) {
	for _, p := range ps {
		if pred(p) {
			n++
		}
	}
	return
}
