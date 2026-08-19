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

// ExplainNext reports what would be handed out next and where everything else stands, each answer
// from the POOL the assignment itself reads — the open tasks for a worker, the unclaimed reviews
// for a reviewer — since a second opinion about eligibility is the drift this exists to expose.
// It answers for an agent, whose own state can rule everything out, or for a ROLE as a hypothetical
// agent of it holding nothing. Both at once is refused: an agent has a role, so they can contradict.
func (e *Engine) ExplainNext(project, agent, role string) (api.NextExplain, error) {
	ps := e.store.For(project)
	if agent != "" && role != "" {
		return api.NextExplain{}, fmt.Errorf("ask about an agent or about a role, not both: %s has a role of its own", agent)
	}
	if agent != "" {
		// An agent brings its own role, so the answer suits the pool it is served from. One the
		// roster never heard of falls through to the backlog question: this reports, it does not gate.
		if a, ok, err := ps.GetAgent(agent); err != nil {
			return api.NextExplain{}, err
		} else if ok {
			role = a.Role
		}
	}
	if role == "" {
		role = "worker" // the backlog question, which is what an unqualified ask has always meant
	}
	out := api.NextExplain{Agent: agent, Role: role}
	switch role {
	case "worker":
	case "reviewer":
		return e.explainReview(project, agent, out)
	case "planner":
		out.RoleNote = "a planner is not served from the backlog: it is briefed, with " +
			"`sindri agent plan <name> <what to plan>` (or B on the Tasks tab), and works that up into a proposal"
		return out, nil
	case "coauthor":
		out.RoleNote = "a coauthor takes no queued work: it shares your checkout and does what you " +
			"tell it, with `sindri agent tell <name> \"…\"`"
		return out, nil
	default:
		return api.NextExplain{}, fmt.Errorf("unknown role %q (worker|reviewer|planner|coauthor)", role)
	}
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
	// Ranked by the assigner's own rule over the same two pools, so the answer to "what is next"
	// cannot part company with what is actually handed out (-> nextUp).
	var pick string
	if t, _, ok := nextUp(packages, leaves, e.tierPrefers(project, agent)); ok {
		pick = t.ID
	}

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
	if e.retired(project, agent) {
		return fmt.Sprintf("retired by the user — `sindri agent retire %s --back` brings it back", agent)
	}
	if e.clearArmed(project, agent) {
		return "a context clear is armed for it — nothing is assigned until it fires"
	}
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

// explainReview is the reviewer's answer: the review that would be picked up, from the same
// UnclaimedReview query reviewDirective hands out from, and why each other PR would not be — the
// states that let one sit unreviewed while a reviewer idled (-> sd-98fa96).
func (e *Engine) explainReview(project, agent string, out api.NextExplain) (api.NextExplain, error) {
	ps := e.store.For(project)
	if agent != "" {
		note, err := reviewHeld(e.store, project, agent)
		if err != nil {
			return out, err
		}
		out.AgentNote = note
	}
	prs, err := ps.PRs()
	if err != nil {
		return out, err
	}
	live, err := ps.LiveReviewPRs()
	if err != nil {
		return out, err
	}
	claimed, err := ps.ActiveReviewers()
	if err != nil {
		return out, err
	}
	var pickID string
	var id int64
	if found, ferr := ps.UnclaimedReview(&id, &pickID); ferr != nil {
		return out, ferr
	} else if !found {
		pickID = ""
	}
	for _, p := range prs {
		if !api.PROpen(p) {
			continue // merged or scrapped: off the board, not an unanswered question
		}
		r := api.PRReason{ID: p.ID, Task: p.Task, Title: e.taskTitle(project, p.Task)}
		switch {
		case p.Status != "open":
			r.Why, r.Note = leftOpen(p)
		case claimed[p.ID] != "":
			r.Why, r.Note = api.ReviewInHand, claimed[p.ID]+" has it"
		case live[p.ID]:
			r.Why = api.ReviewWaiting
		case p.Kind == "interim":
			r.Why, r.Note = api.ReviewInterim, "`sindri pr merge "+p.ID+"` when you want it in"
		default:
			r.Why, r.Note = api.ReviewUnrequested, "`sindri pr review "+p.ID+"`"
		}
		if p.ID == pickID && out.AgentNote == "" {
			pick := r
			out.PickPR = &pick
		}
		out.PRs = append(out.PRs, r)
	}
	return out, nil
}

// leftOpen accounts for a PR past "open" but not yet gone. Each state is a different person's move
// — a rejected one is live work its author resubmits — and each note must name a command that
// WORKS: Merge takes an approved PR and nothing else.
func leftOpen(p store.PR) (api.Reviewability, string) {
	switch p.Status {
	case "approved":
		return api.ReviewApproved, "`sindri pr merge " + p.ID + "`"
	case "rejected":
		return api.ReviewRejected, "it comes back as open when the author submits again"
	case "merging":
		return api.ReviewMerging, "nothing to do — it is going in"
	case "merge-failed":
		// No verb named: the hub died mid-merge, so whether the change reached the base is unknown,
		// and every route back out of this status is refused from it — approve included
		// (api.PRApprovable). Pointing at one would send a confused user straight to an error.
		return api.ReviewMergeFailed, "the merge outcome is unknown — inspect " + p.Base +
			"; `sindri pr info " + p.ID + "` for what happened"
	}
	return api.ReviewSettled, ""
}

// reviewHeld is why a reviewer takes nothing new, "" when it is free. A hold on a PR that has left
// "open" is NOT one: reviewDirective releases it and claims the next review, so trusting the row
// would describe a state the agent's very next ask undoes (a human `pr approve` leaves exactly it).
func reviewHeld(st *store.Store, project, agent string) (string, error) {
	// st's ReviewingPR, not a *ProjectStore's: a pooled reviewer's row is never filed under its
	// own project.
	heldProject, held, err := st.ReviewingPR(project, agent)
	if err != nil || held == "" {
		return "", err
	}
	pr, ok, err := st.For(heldProject).GetPR(held)
	if err != nil {
		return "", err
	}
	if !ok || pr.Status != "open" {
		return "", nil // released on its next ask; it is free in every sense that matters here
	}
	return "holds the review of " + held, nil
}
