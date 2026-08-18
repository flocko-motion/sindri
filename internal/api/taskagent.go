// package: api / taskagent
// type:    logic (who is working a task)
// job:     the one rule for naming the agent behind a task, so the row marker, the detail pane
// and `sindri task list`/`info` all answer from it instead of each looking for
// themselves — three lookups that agree today and drift the first time one is edited.
// limits:  a pure function over the board's agents and PRs; who holds what is the hub's.
package api

// AgentsByTask names the agent behind each task: the one holding it — as the task in hand or as a
// feature container — and failing that the author of the PR waiting on a verdict, since a submitted
// task still has an owner and "who submitted this" is the question a reader opens it with.
//
// A live claim outranks a PR: an agent put back on a rejected task is working it again while its old
// PR is still on the board. Attaching asks a wider question — who can be reached about this task,
// through a subtask levels down — and keeps its own walk for that (-> tui.agentOnTask).
func AgentsByTask(agents []AgentView, prs []PR) map[string]string {
	out := map[string]string{}
	for _, p := range prs { // weakest claim first, so a live one overwrites it
		if p.Task != "" && p.Agent != "" && PROpen(p) {
			out[p.Task] = p.Agent
		}
	}
	for _, a := range agents {
		if a.Feature != "" {
			out[a.Feature] = a.Name
		}
	}
	for _, a := range agents {
		if a.Task != "" {
			out[a.Task] = a.Name
		}
	}
	return out
}

// AgentOnTask names the agent behind one task, "" when nobody is (-> AgentsByTask).
func AgentOnTask(agents []AgentView, prs []PR, id string) string {
	return AgentsByTask(agents, prs)[id]
}
