// package: api / taskagent
// type:    logic (who is working a task)
// job:     the one rule for naming the agent behind a task AND how it is behind it, so the row
// marker, the detail pane and `sindri task list`/`info` all answer from it instead of
// each looking for themselves — three lookups that agree today and drift once one is edited.
// limits:  a pure function over the board's agents and PRs; who holds what is the hub's.
package api

// TaskRelation is HOW an agent stands to a task. Held and worked are separate fields on agent_state
// and a feature sets BOTH at once, so a display that collapses them shows one agent against two rows
// and reads as two agents in one tree — which sent a user hunting a dispatch bug.
type TaskRelation string

const (
	TaskWorking   TaskRelation = "working"   // agent_state.Task: the leaf it is on right now
	TaskHolding   TaskRelation = "holding"   // agent_state.Container: it owns the whole hierarchy
	TaskSubmitted TaskRelation = "submitted" // nobody holds it; this is the PR author awaiting a verdict
)

// TaskHolder is the agent behind a task and which of those it is. The zero value names nobody.
type TaskHolder struct {
	Agent string
	Rel   TaskRelation
}

// AgentsByTask names the agent behind each task: the one holding it — as the task in hand or as a
// feature container — and failing that the author of the PR waiting on a verdict, since a submitted
// task still has an owner and "who submitted this" is the question a reader opens it with.
//
// A live claim outranks a PR: an agent put back on a rejected task is working it again while its old
// PR is still on the board. Working outranks holding on the SAME id, for an agent whose leaf is the
// container itself. Attaching asks a wider question — who can be reached about this task, through a
// subtask levels down — and keeps its own walk for that (-> tui.agentOnTask).
func AgentsByTask(agents []AgentView, prs []PR) map[string]TaskHolder {
	out := map[string]TaskHolder{}
	for _, p := range prs { // weakest claim first, so a live one overwrites it
		if p.Task != "" && p.Agent != "" && PROpen(p) {
			out[p.Task] = TaskHolder{Agent: p.Agent, Rel: TaskSubmitted}
		}
	}
	for _, a := range agents {
		if a.Feature != "" {
			out[a.Feature] = TaskHolder{Agent: a.Name, Rel: TaskHolding}
		}
	}
	for _, a := range agents {
		if a.Task != "" {
			out[a.Task] = TaskHolder{Agent: a.Name, Rel: TaskWorking}
		}
	}
	return out
}

// AgentOnTask names the agent behind one task and how, the zero value when nobody is.
func AgentOnTask(agents []AgentView, prs []PR, id string) TaskHolder {
	return AgentsByTask(agents, prs)[id]
}
