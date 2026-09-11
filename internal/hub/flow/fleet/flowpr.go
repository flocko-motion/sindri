// package: hub/flow/fleet / flowpr
// type:    assembly (a pull request's flow, wired to the hub)
// job:     run the PR map over every live merge intent — read its world, hold its state, and write
// the status each state claims. A PR writes only ITS OWN subject: everything a merge means for the
// agent that filed it or the task it answers is woken, never reached into.
// limits:  the wiring. The map is hub/flow/pr's; merging and scrapping are the verbs' (-> merge.go).
package fleet

import (
	"context"
	"log"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	flowpr "github.com/flo-at/sindri/internal/hub/flow/pr"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// prStatusOf is the status each state claims. Three states share "open" — filed, gating and
// reviewing all mean a live intent — which is exactly why the state is stored separately: the
// status says what the world may see, the state says where the machine has it.
var prStatusOf = map[string]string{
	flowpr.Filed: "open", flowpr.Gating: "open", flowpr.Reviewing: "open",
	flowpr.Approved: "approved", flowpr.Rejected: "open",
	flowpr.Merging: "merging", flowpr.Merged: "merged",
	flowpr.Scrapped: "scrapped", flowpr.Stuck: "merge-failed",
}

// prStateOf is where a PR carrying only a status must start — for one filed before states were
// stored, and for the first pass over a fresh one.
func prStateOf(status string) string {
	switch status {
	case "approved":
		return flowpr.Approved
	case "merging":
		return flowpr.Merging
	case "merged":
		return flowpr.Merged
	case "scrapped":
		return flowpr.Scrapped
	case "merge-failed":
		return flowpr.Stuck
	case "rejected":
		return flowpr.Rejected
	}
	return flowpr.Filed
}

// newPRFlow builds the machine over the PR map. With no tick it is on demand, as the others are.
func (e *Engine) newPRFlow(lifetime context.Context, beat time.Duration) (machine.Machine[flowpr.World], error) {
	return machine.New(lifetime, machine.Config[flowpr.World]{
		States:   flowpr.Flow,
		Start:    flowpr.Start,
		Gather:   e.gatherPR,
		Stored:   e.storedPRState,
		Move:     e.movePRState,
		Do:       e.prDoers(),
		Subjects: e.prSubjects,
		Default:  time.Minute,
		Tick:     beat,
		Record:   prRecorder{e},
	})
}

// prDoers is the implementation of every action a merge intent's states run. A declared action with
// no entry here fails at startup rather than at the merge somebody asked for.
func (e *Engine) prDoers() map[string]machine.Doer[flowpr.World] {
	return map[string]machine.Doer[flowpr.World]{
		flowpr.AskForReview.Name: e.prAct().OpenReviewRow,
		flowpr.DoMerge.Name:      e.prAct().RunMerge,
	}
}

// storedPRState is where the machine has this merge intent, falling back to what its status claims.
func (e *Engine) storedPRState(s string) (string, time.Time, error) {
	project, id, err := subject(s)
	if err != nil {
		return "", time.Time{}, err
	}
	ps := e.Store.For(project)
	state, stamp, err := ps.PRState(id)
	if err != nil {
		return "", time.Time{}, err
	}
	since, _ := time.Parse(time.RFC3339, stamp)
	if state != "" {
		return state, since, nil
	}
	pr, ok, err := ps.GetPR(id)
	if err != nil || !ok {
		return flowpr.Scrapped, since, err
	}
	return prStateOf(pr.Status), since, nil
}

// movePRState writes the state and the status it claims, then WAKES whoever cares. The agent that
// filed it and the task it answers have maps of their own; this one reaches into neither.
func (e *Engine) movePRState(s, from, to, why string) error {
	project, id, err := subject(s)
	if err != nil {
		return err
	}
	ps := e.Store.For(project)
	if err := ps.SetPRState(id, to); err != nil {
		return err
	}
	if want, ok := prStatusOf[to]; ok {
		if pr, found, gerr := ps.GetPR(id); gerr == nil && found && pr.Status != want {
			pr.Status = want
			// The reason a merge intent was discarded is what its author needs, and the board shows
			// it: a PR that simply went "scrapped" left every reader to work out why.
			if to == flowpr.Scrapped {
				pr.Feedback = why
				_ = ps.Log(pr.Agent, "pr-scrapped", id+": "+why)
			}
			if perr := ps.PutPR(pr); perr != nil {
				return perr
			}
		}
	}
	// The milestones are logged as themselves, in the words the lifecycle view reads — a merge that
	// only ever appeared as a state change would drop out of a PR's story entirely.
	switch to {
	case flowpr.Merged:
		_ = ps.LogPR(id, "merged", why)
	case flowpr.Scrapped:
		_ = ps.LogPR(id, "scrapped", why)
	case flowpr.Stuck:
		_ = ps.LogPR(id, "merge-failed", why)
	default:
		_ = ps.LogPR(id, "state", from+" -> "+to+": "+why)
	}
	e.WakeProject(project, prTopicFor(to))
	e.Deps.Notify()
	return nil
}

// prTopicFor is what a merge intent landing in a state means to everybody watching it.
func prTopicFor(state string) machine.Topic {
	switch state {
	case flowpr.Merged:
		return topic.PRMerged
	case flowpr.Gating:
		return topic.GateFinished
	}
	return topic.PRVerdict
}

// gatherPR reads one merge intent's world: what it answers, who is reading it, what was ruled, and
// whether it has landed.
func (e *Engine) gatherPR(s string) (flowpr.World, error) {
	project, id, err := subject(s)
	if err != nil {
		return flowpr.World{}, err
	}
	ps := e.Store.For(project)
	w := flowpr.World{ID: id, Project: project}
	pr, ok, err := ps.GetPR(id)
	if err != nil {
		return w, err
	}
	w.Exists = ok
	if !ok {
		return w, nil
	}
	w.Task, w.Interim = pr.Task, pr.Kind == "interim"
	w.Landed = pr.Status == "merged"
	w.MergeAsked, _ = ps.MergeAsked(id)
	w.Conflicted = flowpr.ConflictStanding(ps, id)
	if live, lerr := ps.LiveReviewPRs(); lerr == nil {
		w.ReviewFiled = live[id]
	}
	// The one git question a merge intent asks, and only while one is running: a merge whose result
	// was lost still settles, because a base already carrying the branch IS merged whatever the
	// action returned. Off the beat for every other PR, which is what keeps this affordable.
	if pr.Status == "merging" && !w.Landed {
		w.Landed = e.baseCarries(project, pr.Branch, pr.Base)
	}
	w.TaskOpen = true
	if t, found, terr := ps.GetTask(pr.Task); terr == nil && found {
		w.TaskOpen = api.Open(t)
	}
	if _, holder := e.prAct().ReviewerHolding(project, id); holder != "" {
		w.Reviewer, w.ReviewOpen = holder, true
	}
	switch pr.Status {
	case "approved":
		w.Verdict = "approved"
	case "rejected":
		w.Verdict = "rejected"
	}
	if pr.Feedback != "" && pr.Status == "open" {
		w.Verdict = "rejected"
	}
	return w, nil
}

// baseCarries reports the branch already sitting on its base — the observation that settles a merge
// nobody heard the end of. A squashed merge leaves no ancestry, so this answers "the branch has
// nothing the base lacks", which a squash satisfies exactly as a fast-forward does.
func (e *Engine) baseCarries(project, branch, base string) bool {
	root := e.Deps.ProjectRoot(project)
	tip, err := git.BranchTip(root, branch)
	if err != nil {
		return false // unreadable: never claim a merge landed on a guess
	}
	return git.IsAncestor(root, tip, base)
}

// unsettled reports a merge intent still worth watching. NOT a status allowlist: "rejected" reads
// like an ending and is not one — its author will answer it — and leaving it off the watch list is
// how a PR against a task that closed underneath went on holding its author.
func unsettled(status string) bool { return status != "merged" && status != "scrapped" }

// prSubjects is every unsettled merge intent. A merged or scrapped one is history: the fleet's grows
// without bound, and re-deciding it every beat would cost more each week it runs.
func (e *Engine) prSubjects() []string {
	prs, err := e.Store.AllPRs()
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range prs {
		if unsettled(p.Status) {
			out = append(out, p.Project+"/"+p.ID)
		}
	}
	return out
}

// LookPR settles one merge intent now, for a caller that has just changed something under it.
func (e *Engine) LookPR(project, id string) {
	if e.prs != nil {
		e.prs.Look(project + "/" + id)
	}
}

// prRecorder keeps a merge intent's own passes on its own history, where a reader already looks.
type prRecorder struct{ e *Engine }

func (r prRecorder) Record(en machine.Entry) {
	project, id, err := subject(en.Subject)
	if err != nil || en.Step == machine.StepLooked {
		return
	}
	detail := en.State + ": " + en.Detail
	if en.Err != nil {
		detail += " — " + en.Err.Error()
	}
	if lerr := r.e.Store.For(project).LogPR(id, "pass", string(en.Step)+": "+detail); lerr != nil {
		log.Printf("hub: %v", lerr)
	}
}

// LookPRs settles every unsettled merge intent in a project, for a caller that wants them straight
// before it reads them.
func (e *Engine) LookPRs(project string) {
	prs, err := e.Store.For(project).PRs()
	if err != nil {
		return
	}
	for _, p := range prs {
		if unsettled(p.Status) {
			e.LookPR(project, p.ID)
		}
	}
}
