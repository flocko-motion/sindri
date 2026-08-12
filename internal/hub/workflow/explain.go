// package: hub/workflow / explain
// type:    logic (why the assigner would, or would not, hand out each task)
// job:     answer "why is nothing being assigned" from the SAME pools the assignment
// itself reads, so the explanation cannot drift from the decision it explains.
// limits:  read-only; it assigns nothing and repairs nothing.
package workflow

import (
	"fmt"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// ExplainNext reports what agent would be handed next and where every other open task stands. The
// claimable set comes from OpenLeaves and OpenContainers themselves — a second opinion about who is
// eligible is exactly the drift this exists to expose. agent may be empty to ask about the backlog
// alone.
func (e *Engine) ExplainNext(project, agent string) (api.NextExplain, error) {
	ps := e.store.For(project)
	out := api.NextExplain{Agent: agent}
	if agent != "" {
		out.AgentNote = e.agentBlocked(ps, project, agent)
	}
	all, err := ps.AllTasks()
	if err != nil {
		return out, err
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		return out, err
	}
	packages, err := ps.OpenContainers()
	if err != nil {
		return out, err
	}
	claimable := map[string]api.Claimability{}
	for _, t := range leaves {
		claimable[t.ID] = api.ClaimableTask
	}
	for _, t := range packages {
		claimable[t.ID] = api.ClaimablePackage
	}
	// The assigner takes a package before a leaf, so the pick follows that order rather than
	// priority across both pools — a P2 package really is handed out before a P1 leaf.
	pick := first(packages, leaves)

	held := map[string]bool{}
	roster, _ := ps.Roster()
	for _, a := range roster {
		st, _ := ps.GetState(a.Name)
		if st.Task != "" {
			held[st.Task] = true
		}
		if st.Container != "" {
			held[st.Container] = true
		}
	}
	released := api.ReleasedByPriority(all)
	openChild, openParent := map[string]bool{}, map[string]bool{}
	byID := map[string]store.Task{}
	for _, t := range all {
		byID[t.ID] = t
	}
	for _, t := range all {
		if t.ParentID != "" && t.Status == "open" {
			openChild[t.ParentID] = true
		}
	}
	for _, t := range all {
		if p, ok := byID[t.ParentID]; ok && p.Status == "open" {
			openParent[t.ID] = true
		}
	}

	for _, t := range all {
		if !api.Open(t) {
			continue
		}
		r := api.TaskReason{ID: t.ID, Title: t.Title, Priority: t.Priority}
		switch {
		case claimable[t.ID] != "":
			r.Why = claimable[t.ID]
		case t.Approval == "pending":
			r.Why, r.Note = api.AwaitingApproval, "`sindri task approve "+t.ID+"`"
		case t.Approval == "rejected":
			r.Why = api.Rejected
		case held[t.ID]:
			r.Why = api.AlreadyHeld
		case !released[t.ID]:
			r.Why, r.Note = api.Unrated, "`sindri task edit "+t.ID+" -p P2`"
		case openParent[t.ID]:
			r.Why = api.InsideAPackage
		case hasAnyChild(all, t.ID) && allChildrenGated(all, t.ID):
			r.Why, r.Note = api.ChildrenGated, "rule on them: `sindri task approve "+t.ID+" --subtasks`"
		default:
			r.Why = api.Stranded
		}
		if pick != "" && t.ID == pick {
			p := r
			out.Pick = &p
		}
		out.Tasks = append(out.Tasks, r)
	}
	if out.AgentNote != "" {
		out.Pick = nil // the backlog is beside the point: this agent takes nothing either way
	}
	return out, nil
}

// agentBlocked reports why an agent can be handed nothing whatever the backlog holds.
func (e *Engine) agentBlocked(ps *store.ProjectStore, project, agent string) string {
	st, err := ps.GetState(agent)
	if err != nil {
		return ""
	}
	if st.Container != "" {
		return fmt.Sprintf("holds feature %s", st.Container)
	}
	if st.Task != "" {
		return fmt.Sprintf("holds %s", st.Task)
	}
	if tokens, full := e.contextFull(project, agent); full {
		return fmt.Sprintf("retired: its context is ~%dk — `sindri agent clear-context %s`", tokens/1000, agent)
	}
	return ""
}

// first returns the id the assigner would take, packages before leaves (-> claimNext).
func first(pools ...[]store.Task) string {
	for _, p := range pools {
		if len(p) > 0 {
			return p[0].ID
		}
	}
	return ""
}

// hasAnyChild reports whether id ever had children, open or not.
func hasAnyChild(all []store.Task, id string) bool {
	for _, t := range all {
		if t.ParentID == id {
			return true
		}
	}
	return false
}

// allChildrenGated reports a package held back by verdicts one level down rather than by anything
// about itself — the remaining way a rated, ungated package can still be offered to nobody.
func allChildrenGated(all []store.Task, id string) bool {
	any := false
	for _, t := range all {
		if t.ParentID != id || !api.Open(t) {
			continue
		}
		any = true
		if t.Approval != "pending" && t.Approval != "rejected" {
			return false
		}
	}
	return any
}
