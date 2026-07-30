// package: hub/workflow / close
// type:    logic (task close/scrap dispatch)
// job:     the two lifecycle-ending verbs every todo backend distinguishes — "done"
// (CloseTask) and "discard" (ScrapTask, optionally down a whole subtree) — routed by the
// task's id prefix to td close/delete, openspec archive/removal, or GitHub issue
// close/delete. Both guard against a live holder first.
// limits:  dispatch only; each op lives in its adapter (td/spec/github).
package workflow

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// CloseTask marks a task done from the task list (host-only) — the "done" close,
// dispatched by backend: a td task closes, an openspec change archives (its deltas
// fold into the specs), a GitHub issue is closed. Allowed even while an agent works
// on it — the agent is freed and told to pick up a new task (see finishTask).
func (e *Engine) CloseTask(project, id string) error { return e.finishTask(project, id, false) }

// ScrapTask discards a task (host-only) — the "discard" close: a td task soft-deletes,
// an openspec change's dir is removed, a GitHub issue is deleted for good. Allowed
// mid-flight (frees the holder). subtree takes everything under the task, done children
// included, since a scrapped parent otherwise strands its subtasks as roots; withPRs
// takes the open PR of each task that goes, so no branch outlives its task. It runs
// deepest first (-> task.Descendants) and stops at the first failure, so a partial
// scrap is a smaller tree, never an orphan under a deleted parent.
func (e *Engine) ScrapTask(project, id string, subtree, withPRs bool) error {
	ps := e.store.For(project)
	var scrapping []string
	if subtree {
		all, err := ps.AllTasks()
		if err != nil {
			return err
		}
		for _, d := range task.Descendants(all, id) {
			scrapping = append(scrapping, d.ID)
		}
	}
	scrapping = append(scrapping, id) // the task itself last, under its own subtree
	var prs []store.PR                // left empty (so the PR pass is a no-op) unless asked for
	if withPRs {
		found, err := ps.PRs()
		if err != nil {
			return err
		}
		prs = found
	}
	for _, victim := range scrapping {
		if err := e.finishTask(project, victim, true); err != nil {
			return fmt.Errorf("scrap %s: %w", victim, err)
		}
		// The PR after its task: ScrapPR leaves the author to the paired close.
		for _, pr := range prs {
			if pr.Task != victim || pr.Status == "merged" || pr.Status == "scrapped" {
				continue
			}
			if err := e.ScrapPR(project, pr.ID); err != nil {
				return fmt.Errorf("scrap PR %s: %w", pr.ID, err)
			}
		}
	}
	return nil
}

// finishTask is the shared close/scrap path: it dispatches to the id's backend for
// either "done" (scrap=false) or "scrap" (scrap=true), then frees any agent that was
// working on the task and pushes it to pick up a new one — cancelling a task
// mid-flight is allowed, never a hard refusal that strands the human.
func (e *Engine) finishTask(project, id string, scrap bool) error {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	// Dispatch the close/scrap to the task's own backend — each source acts only on
	// its own ids, so the workflow never branches on the id scheme. handled flags the
	// owning source; none owning it is a genuinely unknown backend.
	handled := false
	for _, src := range taskSources() {
		ok, err := src.Finish(root, id, scrap)
		if err != nil {
			return err
		}
		if ok {
			handled = true
			break
		}
	}
	if !handled {
		return fmt.Errorf("%s: unknown task backend", id)
	}
	// Free any agent that was on this task and push it to pick up a new one, so a
	// cancelled task doesn't leave a worker grinding on dead work. The worktree is
	// NOT reset here — the agent may keep editing after the push, so cleanup happens
	// when it claims its next task (claimLeaf resets to a clean base then), and the
	// message tells it not to bother cleaning up.
	roster, _ := ps.Roster()
	for _, a := range roster {
		if st, _ := ps.GetState(a.Name); st.Task == id {
			_ = ps.SetState(store.AgentState{Agent: a.Name, Phase: restPhase(a.Role)})
			_ = ps.Log(a.Name, "task-cancelled", id)
			if e.deps.AgentAlive(project, a.Name) {
				// Abort whatever Claude is doing first (ESC), so the cancellation lands
				// on an idle prompt instead of queuing behind in-flight work — "really
				// stop", not just "you'll be told once you're done".
				_ = e.deps.Interrupt(project, a.Name)
				_ = e.deps.InjectWhenReady(project, a.Name, MsgTaskCancelled(id))
			}
		}
	}
	// Reflect the close in the cache directly instead of re-syncing every source. A
	// close changes exactly one task, but SyncTasks refetches td + openspec + the
	// GitHub scan (a network call) — so closing a task blocked for many seconds on
	// work the close didn't need. Update the one row; the next [r]efresh (or the
	// throttled periodic sync) reconciles everything.
	if scrap {
		if derr := ps.RemoveTask(id); derr != nil {
			fmt.Fprintf(os.Stderr, "hub: dropping scrapped task %s from cache: %v\n", id, derr)
		}
	} else if t, ok, gerr := ps.GetTask(id); gerr != nil {
		fmt.Fprintf(os.Stderr, "hub: reading task %s after close: %v\n", id, gerr)
	} else if ok {
		t.Status = "closed"
		if uerr := ps.UpsertTask(t); uerr != nil {
			fmt.Fprintf(os.Stderr, "hub: marking task %s closed in cache: %v\n", id, uerr)
		}
	}
	e.deps.Notify()
	return nil
}
