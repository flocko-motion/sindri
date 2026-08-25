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
	// window, its next assignment clears its context instead of compacting it, before handing the
	// work over — automatic, so Status never needs a word for it. The window is per agent because
	// it is the model's: one number for the fleet would clear 1M agents at 17%.
	ContextTokens int `json:"contextTokens"`
	ContextWindow int `json:"contextWindow"`
	// Model is the raw id its transcript names ("" = not yet observed), the denominator ContextWindow
	// was read against. The account default until a worker can be launched on a chosen one.
	Model string `json:"model,omitempty"`
	// Retired: a human has wound it down, so it is handed no new work while it finishes what it
	// holds. Carried beside Status rather than inside it, because it is true of a busy agent too —
	// that is the whole point of setting it — and Status can only say one thing at a time.
	Retired bool `json:"retired,omitempty"`
	// ClearArmed: a human has armed a context clear, which fires at this agent's next leaf boundary
	// — it is handed no new work in between. Beside Status like Retired, and for the same reason:
	// it is true of a working agent too, and Status can only say one thing at a time.
	ClearArmed bool `json:"clearArmed,omitempty"`
	// UnreadMail is how many messages this agent has not read — a backlog says it has stopped
	// reading, which nothing else surfaces, since a mailbox is content to wait.
	UnreadMail int `json:"unreadMail,omitempty"`
	// Escalation is what an escalated agent asked the user to decide ("" when it is not escalated;
	// Status then reads StatusEscalated). Carried on the board so the question is readable without
	// attaching to the pane — several escalations can be triaged before sitting down with one.
	Escalation string `json:"escalation,omitempty"`
}

// RepoDocState is what a repo has told sindri about itself — its architecture doc and its quality
// gate, each with Advice ("" when fine). Absent, one costs a briefing and the other every submit.
type RepoDocState struct {
	Doc      string `json:"doc"`      // the path in effect: configured, else the default
	Set      bool   `json:"set"`      // the project named it (vs falling back to the default)
	Readable bool   `json:"readable"` // a doc exists at Doc
	Advice   string `json:"advice"`   // "" when nothing to say

	Gate       string `json:"gate"`       // the `verify:` script, "" when the repo declares none
	GateOK     bool   `json:"gateOK"`     // Gate is set and present in the repo
	GateAdvice string `json:"gateAdvice"` // "" when nothing to say
}

// FleetMemory is the machine's memory headroom: what the fleet costs the host now, and the ceiling
// it draws from. Paired on the badge with how many agents are running versus how many exist
// (-> CountRunningAgents) — once the hub can stop an idle one and start a stopped one on demand,
// "how many more fit" answers a question nobody is asking; "is this idle or is it full" is.
type FleetMemory struct {
	UsedBytes  int64 `json:"usedBytes"`
	TotalBytes int64 `json:"totalBytes"`
	// Basis says what UsedBytes counts, which the runtime backend decides: memory containers have
	// taken as they used it, or memory each pod reserved up front and holds whether it uses it.
	Basis string `json:"basis,omitempty"`
}

// Known reports whether the runtime answered at all; an unknown figure is rendered as nothing
// rather than as an empty machine.
func (m FleetMemory) Known() bool { return m.TotalBytes > 0 }

// FreeBytes is what is left for new agents, never negative: an overcommitted host has none free.
func (m FleetMemory) FreeBytes() int64 {
	if free := m.TotalBytes - m.UsedBytes; free > 0 {
		return free
	}
	return 0
}

// BoardState is the whole board: Agents and PRs global, Tasks only the selected project's.
type BoardState struct {
	Agents   []AgentView             `json:"agents"`
	Tasks    []Task                  `json:"tasks"`
	PRs      []PR                    `json:"prs"`
	Runs     []Run                   `json:"runs,omitempty"`
	Projects []Project               `json:"projects"`
	Orphans  []string                `json:"orphans"`   // pods with no roster entry (D14)
	Chat     ChatView                `json:"chat"`      // the user's chatroom: members + transcript
	RepoDocs map[string]RepoDocState `json:"repo_docs"` // per repo tag: its architecture doc + any gap
	// RuntimeHint explains an unreachable container runtime, "" when it answers. It rides on the
	// board because the hub already knows: its liveness sweep lists pods every couple of seconds, so
	// a front-end that probed for itself paid seconds to learn what this says for free.
	RuntimeHint string `json:"runtime_hint,omitempty"`
	// SpecCLIMissing: an openspec/ folder exists but the hub found no openspec CLI on its PATH.
	SpecCLIMissing bool `json:"spec_cli_missing"`
	// StartedAt is when this hub process came up (RFC3339), so `hub status` reads uptime from the board.
	StartedAt string `json:"started_at"`
	// DefaultMemory is the RAM an agent gets with none configured — the runtime's own current default.
	DefaultMemory string `json:"defaultMemory"`
	// Memory is the machine's memory headroom for agents. It sits here beside DefaultMemory and
	// StartedAt because it is a property of the host and its runtime rather than of any project,
	// and it rides on the board so a front-end renders the figure instead of measuring the host.
	Memory FleetMemory `json:"memory"`
	// Sections are the dashboard's tabs as the hub resolved them against this very board: which
	// views exist, and the badge each shows. They ride on the board so a front-end renders the
	// counts instead of deciding them (-> SectionAttention).
	Sections []Section `json:"sections,omitempty"`
	// Mail is the newest messages agents must read, fleet-wide, newest first — a WINDOW, with each
	// body cut to a preview. MailTotal and MailUnread count the whole mailbox, so a view says
	// "showing the last N of M" rather than presenting a window as the history (-> MailWindow).
	Mail       []Mail `json:"mail,omitempty"`
	MailTotal  int    `json:"mailTotal"`
	MailUnread int    `json:"mailUnread"`
	// MailUnreadUser is unread mail addressed to the USER, across every repo — the only part of the
	// mailbox that can ask a person for anything. Fleet-wide, since they are the same person in each.
	MailUnreadUser int `json:"mailUnreadUser"`
	// MailUnreadByRepo is unread mail per repo tag, for a view scoped to one repo — counted over the
	// whole mailbox like the totals, not over the window.
	MailUnreadByRepo map[string]int `json:"mailUnreadByRepo,omitempty"`
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
	case "", "down", "stopped", StatusUnknown, "launching", "stopping", StatusLaunchFailed:
		return true
	}
	return false
}

// AgentNeedsLaunch reports whether an agent has no pod and none on the way — narrower than
// AgentNotUp, which also covers one in flight. "stopped" and a failed launch count too.
func AgentNeedsLaunch(status string) bool {
	return status == "down" || status == "stopped" || status == StatusUnknown || status == StatusLaunchFailed
}

// ClearWaitsFor says what an armed context clear will fire AFTER: the id of the work in hand, or ""
// when the agent is already at a leaf boundary and the clear lands at once. The rule, so both
// front-ends state the same "when" — a leaf task defers it, a held feature does not (the clear
// fires between subtasks), and a reviewer's open review does. It mirrors the hub's own boundary
// test, which reads the same two facts from the store.
func ClearWaitsFor(a AgentView) string {
	if a.Task != "" {
		return a.Task
	}
	if a.Role == "reviewer" && a.PR != "" {
		return a.PR
	}
	return ""
}

// These satisfy commands.Board — the dashboard's badge counts.

// OpenTaskCount is the number of not-done tasks in the selected project.
func (b BoardState) OpenTaskCount() int { return countTasks(b.Tasks, Open) }

// AgentCount is the whole roster size (down agents are still agents).
func (b BoardState) AgentCount() int { return len(b.Agents) }

// RunningAgentCount is how many of the roster currently have a pod up — the other half of the
// headroom badge's "is this idle or full" question, paired with AgentCount.
func (b BoardState) RunningAgentCount() int {
	n := 0
	for _, a := range b.Agents {
		if !AgentNotUp(a.Status) {
			n++
		}
	}
	return n
}

// OpenPRCount is open PRs across the fleet (neither merged nor scrapped), matching the PRs tab default.
func (b BoardState) OpenPRCount() int { return countPRs(b.PRs, PROpen) }

// OpenRunCount is queued or running runs across the fleet, matching the Runs tab default.
func (b BoardState) OpenRunCount() int { return countRuns(b.Runs, RunOpen) }

// RepoCount is the number of repos the hub tracks.
func (b BoardState) RepoCount() int { return len(b.Projects) }

// ChatMemberCount is the number of agents in the user's chatroom.
func (b BoardState) ChatMemberCount() int { return len(b.Chat.Members) }

// TasksAwaitingVerdictCount is how much work the approval gate holds until the user rules on it.
func (b BoardState) TasksAwaitingVerdictCount() int { return CountAwaitingVerdict(b.Tasks) }

// TasksNeedingUserCount is the Tasks section's attention count: tasks stopped behind either gate,
// approval or rating (-> TaskNeedsUser). Wider than the verdict count on purpose — an unrated task
// is exactly as unclaimable as an unapproved one, and the badge and the red row it colours are one
// claim, so both read this.
func (b BoardState) TasksNeedingUserCount() int { return CountTasksNeedingUser(b.Tasks) }

// AgentsNeedingUserCount is the Agents section's attention count: agents that cannot move until a
// human acts (-> AgentNeedsUser).
func (b BoardState) AgentsNeedingUserCount() int { return CountAgentsNeedingUser(b.Agents) }

// PRsNeedingUserCount is the PRs section's attention count: PRs waiting on a merge, or on a review
// no live reviewer will give (-> PRNeedsUser). It reads the roster too, since who is running is
// half the question.
func (b BoardState) PRsNeedingUserCount() int { return CountPRsNeedingUser(b.PRs, b.Agents) }

// UnreadUserMailCount is the Mail section's ATTENTION count. Only the user's own: the rest of the
// mailbox is agent traffic, and a marker over that would be permanently lit and instantly ignored.
func (b BoardState) UnreadUserMailCount() int { return b.MailUnreadUser }

// UnreadMailCount is the Mail section's badge: unread across the whole mailbox, not the window,
// since a badge that stopped rising once the history outgrew the window would say the wrong thing
// exactly when there was most to say.
func (b BoardState) UnreadMailCount() int { return b.MailUnread }

// SectionAttention is how many rows of the named section wait on the user, read off the sections
// the hub resolved. A front-end asks by key so every tab is drawn by the same line of code; a
// board with no sections on it (an older hub, a hand-built snapshot) marks nothing rather than
// deriving a count of its own.
func (b BoardState) SectionAttention(key string) int {
	for _, s := range b.Sections {
		if s.Key == key {
			return s.Attention
		}
	}
	return 0
}

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

func countRuns(rs []Run, pred func(Run) bool) (n int) {
	for _, r := range rs {
		if pred(r) {
			n++
		}
	}
	return
}
