// package: hub / commentscope
// type:    logic (which tasks a reviewer may write a finding onto)
// job:     answer "what can this reviewer comment on" once, for the gate that offers the verb,
// the resolution of an id-less comment, and the check on an explicit id — three readers
// that disagreeing would let the verb be offered and then refuse everything.
// limits:  the reviewer's scope only; the other roles' is their own state (-> commentTarget).
package hub

import (
	"fmt"

	"github.com/flo-at/sindri/internal/hub/registry"
)

// reviewerTasks is what a reviewer may comment on, newest first: the task of the review it HOLDS,
// then the task of every PR it has ruled on. Having ruled is proof it read that work, which is what
// the narrow gate was really asking — and a verdict used to END the reviewer's reach at the moment
// it acquired afterthoughts, leaving mail as the only channel and no record on the task.
func (h *Hub) reviewerTasks(c registry.Caller) ([]string, error) {
	ps := h.store.For(c.Project)
	held, err := ps.ReviewingPR(c.Agent)
	if err != nil {
		return nil, err
	}
	ruled, err := ps.RuledPRs(c.Agent)
	if err != nil {
		return nil, err
	}
	prs := ruled
	if held != "" {
		prs = append([]string{held}, ruled...)
	}
	var out []string
	seen := map[string]bool{}
	for _, id := range prs {
		p, ok, gerr := ps.GetPR(id)
		if gerr != nil {
			return nil, gerr
		}
		if !ok {
			if id == held { // an assigned PR with no row is a hub fault, not the agent's to work around
				return nil, fmt.Errorf("agent %q is reviewing PR %q, which is not in the store", c.Agent, id)
			}
			continue // an old verdict whose PR is gone: skip it rather than fail the whole verb
		}
		if p.Task == "" || seen[p.Task] {
			continue
		}
		seen[p.Task] = true
		out = append(out, p.Task)
	}
	return out, nil
}

// replyNothingRuledOn refuses the verb to a reviewer with nothing in reach. It names what WOULD be
// reachable, since a reviewer holding no review still has everything it has ruled on.
const replyNothingRuledOn = "You have no review in hand and haven't ruled on a PR yet, so there's " +
	"no task to comment on. Once you approve or reject one, its task stays open to you."
