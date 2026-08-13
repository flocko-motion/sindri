// package: hub/workflow / adopt
// type:    logic (a task that gains a child while someone is working it)
// job:     grow an agent's unit of work when a child is added under the task it holds —
// the leaf becomes a FEATURE held by the same agent on the same branch, so the
// PR waits for the whole hierarchy instead of merging over the new work.
// limits:  the promotion and the notice; assignment inside a feature is feature.go's
// (advanceContainer/resumeContainer) and the merge guard is merge.go's.
package workflow

import (
	"github.com/flo-at/sindri/internal/hub/store"
)

// adoptChild is what happens when a task gains a child while an agent is working it: the agent
// KEEPS the work and its unit grows to the hierarchy. It is told either way — its unit changed
// shape by someone else's act, and being refused at the next checkpoint is not how it should find
// out. An agent with a PR already out is not moved (-> MergePR raises that bar instead).
func (e *Engine) adoptChild(project, parent, child string) {
	if parent == "" || child == "" {
		return
	}
	ps := e.store.For(project)
	roster, err := ps.Roster()
	if err != nil {
		return
	}
	for _, a := range roster {
		st, _ := ps.GetState(a.Name)
		if st.Container != parent && st.Task != parent {
			continue
		}
		promoted := st.Container == "" && st.Phase == "working"
		if promoted {
			e.promoteToFeature(project, a.Name, parent)
		}
		if e.deps.AgentAlive(project, a.Name) {
			_ = e.deps.InjectWhenReady(project, a.Name, MsgTaskGainedChild(parent, child, promoted))
		}
		return
	}
}

// promoteToFeature moves an agent from holding a task as a LEAF to holding it as a FEATURE: same
// agent, same branch — a leaf branch is already named for its task — one PR at the end. It assigns
// nothing: the directive picks the subtask, being the one place that asks every question about what
// may go out, and it asks LATER — a proposal's approval row is written a moment after the task.
func (e *Engine) promoteToFeature(project, agent, task string) {
	ps := e.store.For(project)
	_ = ps.SetState(store.AgentState{Agent: agent, Container: task, Branch: task, Phase: "idle"})
	_ = ps.Log(agent, "promote", task+" gained work, so it is a feature now")
	e.deps.Notify()
}
