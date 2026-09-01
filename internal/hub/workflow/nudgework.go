// package: hub/workflow / nudgework
// type:    logic (waking an idle worker toward claimable backlog work)
// job:     the event-triggered and periodic sweeps that push an idle worker toward work it
// hasn't been told about, and the feature-worker equivalent between subtasks — never a
// claim, since that stays the worker's own next ask.
// limits:  the wake and its dedup (last_nudge); which task an agent may take is nextUp's.
package workflow

import "github.com/flo-at/sindri/internal/hub/store"

// nudgeIdleWorkers tells idle workers the instant rated work exists (AssignPendingWork is the periodic
// backstop) — each pick removed from the pool first, so a herd isn't all told the same task.
func (e *Engine) nudgeIdleWorkers(project, priority string) {
	if priority == "" {
		return
	}
	ps := e.store.For(project)
	packages, err := ps.OpenContainers()
	if err != nil {
		return
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		return
	}
	roster, err := ps.Roster()
	if err != nil {
		return
	}
	open := openTaskIDs(packages, leaves)
	for _, a := range roster {
		if len(packages) == 0 && len(leaves) == 0 {
			return // nothing left to offer whoever is left in the roster
		}
		// Any RUNNING agent: this fires the instant work is rated, and a message typed behind a busy
		// turn still lands when it ends. The periodic backstop waits for an idle prompt instead.
		if !e.deps.AgentUp(project, a.Name) || e.allowed(project, a.Name).Nudge != "" {
			continue
		}
		st, _ := ps.GetState(a.Name)
		e.forgetStaleNudge(ps, a.Name, st.LastNudge, open)
		t, isPackage, ok := nextUp(packages, leaves, e.tierPrefers(project, a.Name))
		if !ok {
			continue
		}
		if !e.notifyOnce(ps, project, a.Name, st.LastNudge, t.ID) {
			continue // not actually pushed — leave the task offered to whoever else is idle
		}
		if isPackage {
			packages = withoutTask(packages, t.ID)
		} else {
			leaves = withoutTask(leaves, t.ID)
		}
	}
}

// openTaskIDs is every id OpenLeaves/OpenContainers would still offer — unclaimed, rated, ungated —
// checked against an agent's last_nudge instead of the pool this sweep shrinks as it hands work out.
func openTaskIDs(packages, leaves []store.Task) map[string]bool {
	ids := make(map[string]bool, len(packages)+len(leaves))
	for _, t := range packages {
		ids[t.ID] = true
	}
	for _, t := range leaves {
		ids[t.ID] = true
	}
	return ids
}

// forgetStaleNudge clears last_nudge once its task drops out of that offerable set for ANY reason —
// claimed by someone else, closed, gated. Left set, a task that leaves and later returns under the
// same id (reopened, or claimed then released) would match the memory on sight and go unannounced.
func (e *Engine) forgetStaleNudge(ps *store.ProjectStore, agent, lastNudge string, open map[string]bool) {
	if lastNudge != "" && !open[lastNudge] {
		_ = ps.SetLastNudge(agent, "")
	}
}

// notifyOnce pushes MsgWorkAvailable at most once per task id per agent, and reports whether it
// actually landed, so a caller does not consume the pool slot for a skip.
func (e *Engine) notifyOnce(ps *store.ProjectStore, project, agent, lastNudge, taskID string) bool {
	if lastNudge == taskID {
		return false
	}
	// Recorded only once Deliver says it landed — a signed-out pane or a container mid-restart reports
	// "up" but never receives it, and marking it told anyway would silence the backstop for good.
	if err := e.deps.Deliver(project, agent, MsgWorkAvailable(taskID), PushOnly); err != nil {
		return false
	}
	_ = ps.Log(agent, "nudge", "work available: "+taskID)
	_ = ps.SetLastNudge(agent, taskID)
	return true
}

// withoutTask drops one task by id, preserving order — how nudgeIdleWorkers simulates a pool
// shrinking as it hands each eligible agent, in turn, whatever is left in it.
func withoutTask(tasks []store.Task, id string) []store.Task {
	for i, t := range tasks {
		if t.ID == id {
			out := make([]store.Task, 0, len(tasks)-1)
			out = append(out, tasks[:i]...)
			return append(out, tasks[i+1:]...)
		}
	}
	return tasks
}

// AssignPendingWork nudges every idle worker toward claimable work it hasn't been told about — a push
// only, since claiming here on the agent's behalf could race its own claim and strand it in_progress.
func (e *Engine) AssignPendingWork(project string) {
	_ = e.SyncTasks(project) // best-effort refresh; cached set on failure, same as claimNext's own read
	ps := e.store.For(project)
	packages, err := ps.OpenContainers()
	if err != nil {
		return
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		return
	}
	roster, err := ps.Roster()
	if err != nil {
		return
	}
	open := openTaskIDs(packages, leaves)
	for _, a := range roster {
		// An IDLE prompt, unlike the event-triggered sweep: this repeats every tick, so it waits for a
		// moment the agent is actually listening rather than queuing behind whatever it is doing.
		if !e.deps.AgentIdle(project, a.Name) {
			continue
		}
		st, _ := ps.GetState(a.Name)
		if st.Phase != "" && st.Phase != "idle" {
			continue // mid some other flow — leave it alone
		}
		// The same question nudgeIdleWorkers asks, which is the whole point: this sweep was written
		// three lines from that one without the guard, and pushed a retired agent "ready for you"
		// every thirty seconds (-> sd-daf9c7). A held feature is the one case where a wake still has
		// something to say, so it is asked first.
		if st.Container != "" {
			if e.allowed(project, a.Name).Wake == "" {
				e.assignPendingSubtask(project, a.Name, st.Container)
			}
			continue
		}
		if e.allowed(project, a.Name).Nudge != "" {
			continue
		}
		e.forgetStaleNudge(ps, a.Name, st.LastNudge, open)
		t, isPackage, ok := nextUp(packages, leaves, e.tierPrefers(project, a.Name))
		if !ok {
			continue
		}
		if !e.notifyOnce(ps, project, a.Name, st.LastNudge, t.ID) {
			continue // not actually pushed — leave the task offered to whoever else is idle
		}
		if isPackage {
			packages = withoutTask(packages, t.ID)
		} else {
			leaves = withoutTask(leaves, t.ID)
		}
	}
}

// assignPendingSubtask wakes a feature worker once an approval or rejection clears its next
// subtask — a push toward asking again, not a claim, for the same reason AssignPendingWork itself.
func (e *Engine) assignPendingSubtask(project, agent, container string) {
	ps := e.store.For(project)
	st, _ := ps.GetState(agent)
	if st.Task != "" {
		return // already holding a subtask
	}
	children, err := ps.OpenSubtasks(container)
	if err != nil {
		return // the check failed — try again next sweep
	}
	e.forgetStaleNudge(ps, agent, st.LastNudge, openTaskIDs(children, nil)) // even when nothing is open
	if len(children) == 0 {
		return // still gated — nothing claimable yet
	}
	e.notifyOnce(ps, project, agent, st.LastNudge, children[0].ID)
}
