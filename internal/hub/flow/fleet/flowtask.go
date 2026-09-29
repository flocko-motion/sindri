// package: hub/flow/fleet / flowtask
// type:    assembly (a task's flow, wired to the hub)
// job:     run the task map over every active task — read a task's world, translate between its
// declared state and the status its source keeps, and write that status when it moves.
// limits:  the wiring. The map is hub/flow/task's; where a status lives is SetStatus's.
package fleet

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	flowtask "github.com/flo-at/sindri/internal/hub/flow/task"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// statusOf is the status a state claims, and stateOf its inverse. The task's stored state IS its
// status: there is no second column to drift from it, which is what "one source owns a task" means
// here — a state written is a status written, at whichever source keeps it.
var statusOf = map[string]string{
	flowtask.Open:      "open",
	flowtask.Held:      "in_progress",
	flowtask.Reviewing: "in_review",
	flowtask.Closed:    "closed",
}

// stateOf maps a status back onto the state it means. A status nothing here names — a source's own
// word for something — leaves the task open, which claims nothing about it.
func stateOf(t store.Task) string {
	if t.Approval == "pending" {
		return flowtask.Proposed // the gate stands ahead of whatever the status says
	}
	switch {
	case !api.Open(t):
		return flowtask.Closed
	case t.Status == "in_review":
		return flowtask.Reviewing
	case t.Status == "in_progress":
		return flowtask.Held
	}
	return flowtask.Open
}

// newTaskFlow builds the machine over the task map. With no tick it is on demand, as the agent's is.
func (e *Engine) newTaskFlow(lifetime context.Context, beat time.Duration) (machine.Machine[flowtask.World], error) {
	return machine.New(lifetime, machine.Config[flowtask.World]{
		States:   flowtask.Flow,
		Start:    flowtask.Start,
		Gather:   e.gatherTask,
		Stored:   e.storedTaskState,
		Move:     e.moveTaskState,
		Subjects: e.taskSubjects,
		Default:  10 * time.Minute,
		Tick:     beat,
		Record:   taskRecorder{e},
	})
}

// storedTaskState is the state a task's own status already claims.
func (e *Engine) storedTaskState(s string) (string, time.Time, error) {
	project, id, err := subject(s)
	if err != nil {
		return "", time.Time{}, err
	}
	t, ok, err := e.Store.For(project).GetTask(id)
	if err != nil || !ok {
		return flowtask.Closed, time.Time{}, err // gone from the cache reads as finished until it returns
	}
	since, _ := time.Parse(time.RFC3339, t.UpdatedAt)
	return stateOf(t), since, nil
}

// moveTaskState writes the status the new state claims, at whichever source owns this task. A state
// that claims no status — the approval gate — moves the gate instead.
func (e *Engine) moveTaskState(s, from, to, why string) error {
	project, id, err := subject(s)
	if err != nil {
		return err
	}
	if from == flowtask.Proposed && to == flowtask.Closed {
		return e.Store.For(project).SetApproval(id, "rejected", why)
	}
	want, ok := statusOf[to]
	if !ok {
		return nil // the proposal gate is entered by the planner, never by an observation
	}
	if err := e.prAct().SetStatus(project, id, want); err != nil {
		return err
	}
	_ = e.taskAct().RefreshTask(project, id)
	e.WakeProject(project, taskTopicFor(from, to))
	return nil
}

// taskTopicFor is what a task landing in a state means to everybody watching it. It takes where the
// task CAME FROM as well: a proposal opening is the approval gate opening, which is a different
// event from a task merely being claimable — it is what frees a feature holder standing in front of
// gated children (-> cond.SubtasksGated), and that is the only thing watching for it.
func taskTopicFor(from, to string) machine.Topic {
	switch {
	case to == flowtask.Closed:
		return topic.TaskClosed
	case from == flowtask.Proposed && to == flowtask.Open:
		return topic.TaskApproved
	}
	return topic.TaskAvailable
}

// gatherTask reads one task's world: what it claims, who holds it, what stands against it, and
// whether anything is left beneath it.
func (e *Engine) gatherTask(s string) (flowtask.World, error) {
	project, id, err := subject(s)
	if err != nil {
		return flowtask.World{}, err
	}
	ps := e.Store.For(project)
	w := flowtask.World{ID: id, Project: project}
	t, ok, err := ps.GetTask(id)
	if err != nil {
		return w, err
	}
	w.Exists = ok
	if !ok {
		return w, nil
	}
	w.Pending = t.Approval == "pending"
	w.Refused = t.Approval == "rejected"
	facts, err := e.taskAct().TaskReality(project, id)
	if err != nil {
		return w, err
	}
	w.ActivePR, w.Landed, w.OpenChildren = facts.ActivePR, facts.MergedFinalPR, facts.OpenChildren
	if facts.Assigned {
		w.Holder = holderOf(ps, id)
	}
	return w, nil
}

// holderOf names the agent holding a task, "" when none does.
func holderOf(ps *store.ProjectStore, id string) string {
	roster, err := ps.Roster()
	if err != nil {
		return ""
	}
	for _, a := range roster {
		if st, serr := ps.GetState(a.Name); serr == nil && (st.Task == id || st.Container == id) {
			return a.Name
		}
	}
	return ""
}

// taskSubjects is every ACTIVE task, as "project/id". Closed ones are left out: a fleet's history
// grows without bound and re-deciding it every beat would cost more each week it runs.
func (e *Engine) taskSubjects() []string {
	projects, err := e.Store.Projects()
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range projects {
		tasks, terr := e.Store.For(p.Tag).AllTasks()
		if terr != nil {
			continue
		}
		for _, t := range tasks {
			if api.Open(t) {
				out = append(out, p.Tag+"/"+t.ID)
			}
		}
	}
	return out
}

// LookTask runs one pass over a task now, for a caller that wants its status settled before it goes
// on — a list, a board read, or a verb that has just changed something under it.
func (e *Engine) LookTask(project, id string) {
	if e.tasks != nil {
		e.tasks.Look(project + "/" + id)
	}
}

// LookTasks settles every active task in a project. It replaces the repair sweep: the checks that
// sweep made by hand are the task map's own conditions now.
func (e *Engine) LookTasks(project string) {
	tasks, err := e.Store.For(project).AllTasks()
	if err != nil {
		return
	}
	for _, t := range tasks {
		if api.Open(t) {
			e.LookTask(project, t.ID)
		}
	}
}

// taskRecorder keeps a task's own passes on the record, under the id that pass carries.
type taskRecorder struct{ e *Engine }

func (r taskRecorder) Record(en machine.Entry) {
	project, id, err := subject(en.Subject)
	if err != nil {
		return
	}
	detail := en.Detail
	if en.State != "" {
		detail = en.State + ": " + detail
	}
	if en.Err != nil {
		detail += " — " + en.Err.Error()
	}
	if strings.TrimSpace(detail) == "" {
		return
	}
	if lerr := r.e.Store.For(project).LogPass("task:"+id, string(en.Step), en.Pass, detail); lerr != nil {
		log.Printf("hub: %v", lerr)
	}
}
