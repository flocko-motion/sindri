// package: hub/flow/fleet / flowsay
// type:    logic (the words behind what a state says)
// job:     render what an agent is told where it stands — the state names the speech, this turns it
// into the agent's own words, and appends the verbs that state offers.
// limits:  rendering. WHICH speech belongs to which state is the flow's declaration.
package fleet

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/prompts"
	hubtask "github.com/flo-at/sindri/internal/hub/task"
	"strings"

	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	"github.com/flo-at/sindri/internal/hub/flow/says"
)

// speak renders one state's Says into the agent's own words. A speech with no words here is a loud
// failure: an agent left with no answer at all is the one outcome worse than a wrong one.
func (e *Engine) speak(w flow.World, s flow.State) (string, error) {
	switch s.Says {
	case says.Escalated:
		return prompts.DirEscalated(w.Escalation), nil
	case says.Retired:
		return prompts.DirRetired, nil
	case says.Coauthor:
		return prompts.DirCoauthor, nil
	case says.Planner:
		return prompts.DirPlanner, nil
	case says.Planning:
		return prompts.DirPlanning, nil
	case says.AwaitVerdict:
		return prompts.DirSubmitted, nil
	case says.Gating:
		return prompts.DirGating, nil
	case says.NoTasks:
		return prompts.DirNoTasks, nil
	case says.NoReviews:
		return prompts.DirNoReviews, nil
	case says.Preparing:
		return prompts.DirBusy(s.Name), nil
	case says.Mail:
		return prompts.DirMailWaiting, nil
	case says.Stalled:
		return prompts.DirStalled(w.Task, w.StillFor), nil
	case says.FeatureDone:
		return prompts.DirContainerDone(w.Container), nil
	case says.FeatureGated:
		return prompts.ReplyFeatureGated(w.Container, hubtask.OpenIDs(w.Gated)), nil
	case says.Resolving:
		return prompts.DirResolving(w.Task), nil
	case says.Reviewing:
		return e.reviewWords(w), nil
	case says.Working:
		if w.Container != "" {
			return prompts.DirContainerWorking(w.Container, w.Task, w.Aim, w.Ceiling), nil
		}
		return prompts.DirWorking(w.Task, w.Aim, w.Ceiling), nil
	case says.Rejected:
		return e.rejectionWords(w), nil
	}
	return "", fmt.Errorf("workflow: %s says %q, which has no words", s.Name, s.Says)
}

// rejectionWords hands back the feedback standing against the work in hand, or against a PR the
// agent has out while holding nothing else.
func (e *Engine) rejectionWords(w flow.World) string {
	v, task := w.Held, w.Task
	if !v.Rejected {
		v, task = w.Awaiting, w.AwaitingTask
	}
	if w.Container != "" {
		return prompts.DirContainerRejected(w.Container, task, v.Feedback, v.Round, w.Aim, w.Ceiling)
	}
	return prompts.DirRejected(task, v.Feedback, v.Round, w.Aim, w.Ceiling)
}

// reviewWords restates the one PR a reviewer holds.
func (e *Engine) reviewWords(w flow.World) string {
	heldProject, held, err := e.Store.ReviewingPR(w.Project, w.Name)
	if err != nil || held == "" {
		return prompts.DirNoReviews
	}
	pr, ok, perr := e.Store.For(heldProject).GetPR(held)
	if perr != nil || !ok {
		return prompts.DirNoReviews
	}
	return prompts.DirReview(pr.ID, pr.Task, e.prAct().TaskTitle(heldProject, pr.Task), pr.Agent,
		e.Deps.ArchitectureDoc(heldProject))
}

// offerLines lists what the agent may run where it stands, so the answer to "where am I" carries the
// answer to "what can I do" — the two questions an agent asks together.
func offerLines(offers []machine.Offer) string {
	if len(offers) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nAvailable to you here:")
	for _, o := range offers {
		fmt.Fprintf(&b, "\n  %-12s %s", o.Verb.Name, o.Verb.Help)
	}
	return b.String()
}
