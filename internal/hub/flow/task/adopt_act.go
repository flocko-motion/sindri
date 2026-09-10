// package: hub/flow/task / adopt_act
// type:    logic (a task that gains a child while someone is working it)
// job:     grow an agent's unit of work when a child is added under the task it holds —
// the leaf becomes a FEATURE held by the same agent on the same branch, so the
// PR waits for the whole hierarchy instead of merging over the new work.
// limits:  the promotion and the notice; assignment inside a feature is feature_act.go's
// (AdvanceContainer/ResumeContainer) and the merge guard is hub/flow/pr's.
package task

import (
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// AdoptChild is what happens when a task gains a child while an agent is working it: the agent
// KEEPS the work and its unit grows to the hierarchy. It is told either way — its unit changed
// shape by someone else's act, and being refused at the next checkpoint is not how it should find
// out. An agent with a PR already out is not moved (-> MergePR raises that bar instead).
func (a *Act) AdoptChild(project, parent, child string) {
	if parent == "" || child == "" {
		return
	}
	ps := a.Store.For(project)
	roster, err := ps.Roster()
	if err != nil {
		return
	}
	// Every agent that matches, never the first: one task cannot be two agents' at once, but that is
	// an invariant of the claim queries rather than of this loop, and assuming it here silently is
	// how one of them would come to be left out (-> tellHolder, which holds the same line).
	for _, ag := range roster {
		st, _ := ps.GetState(ag.Name)
		if st.Container != parent && st.Task != parent {
			continue
		}
		promoted := st.Container == "" && st.Phase == "working"
		if promoted {
			a.PromoteToFeature(project, ag.Name, parent)
		}
		// Mail-only, and no liveness gate: it MUST read this, and one that was down when the child
		// landed would otherwise find out by being refused at its next checkpoint.
		_ = a.Harness.Say(project, ag.Name, prompts.MsgTaskGainedChild(parent, child, promoted), mail.MailOnly)
	}
}

// PromoteToFeature moves an agent from holding a task as a LEAF to holding it as a FEATURE: same
// agent, same branch — a leaf branch is already named for its task — one PR at the end. It assigns
// nothing: the directive picks the subtask, being the one place that asks every question about what
// may go out, and it asks LATER — a proposal's approval row is written a moment after the task.
func (a *Act) PromoteToFeature(project, agent, task string) {
	ps := a.Store.For(project)
	_ = ps.SetState(store.AgentState{Agent: agent, Container: task, Branch: task, Phase: "idle"},
		store.ReasonClaimed, "promoted to a feature: "+task)
	_ = ps.Log(agent, "promote", task+" gained work, so it is a feature now")
	a.Deps.Notify()
}
