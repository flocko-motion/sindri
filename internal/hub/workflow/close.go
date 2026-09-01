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
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/task"
)

// CloseTask is the "done" close, dispatched by backend: td closes, openspec archives, GitHub closes
// the issue. Allowed mid-flight — the agent is freed and told to take a new task.
func (e *Engine) CloseTask(project, id string) error { return e.finishTask(project, id, false) }

// ScrapTask is the "discard" close, each source deleting its own. subtree takes everything under the
// task (a scrapped parent otherwise strands its subtasks as roots) and withPRs each one's open PR.
// Deepest first, stopping at the first failure, so a partial scrap is a smaller tree.
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

// finishAtSource ends a task where its status actually lives. Each source acts only on its own ids,
// so this never branches on the id scheme; nothing owning it means a genuinely unknown backend.
func (e *Engine) finishAtSource(project, root, id string, scrap bool) error {
	for _, src := range e.taskSources(project) {
		ok, err := src.Finish(root, id, scrap)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return fmt.Errorf("%s: unknown task backend", id)
}

// recordEnded writes an ending sindri must remember itself: a status living in the REPO never moves
// for the hub, since the worker's tick is on its BRANCH. The override survives the sync's rebuild.
func (e *Engine) recordEnded(project, id string) error {
	ps := e.store.For(project)
	if ps.OwnsTask(id) {
		return nil
	}
	if err := ps.SetClosedOverride(id); err != nil {
		return err
	}
	t, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		return err
	}
	t.Status = "closed"
	return ps.UpsertTask(t)
}

// SetStatus moves a task to want without the caller knowing where that status lives: done goes
// through the owning source, anything else is sindri's own scheduling. Every site remembering for
// itself is what left openspec tasks open over finished work, four times.
func (e *Engine) SetStatus(project, id, want string) error {
	ps := e.store.For(project)
	if (task.Task{Status: want}).IsClosed() {
		if err := e.finishAtSource(project, e.deps.ProjectRoot(project), id, false); err != nil {
			return err
		}
		return e.recordEnded(project, id)
	}
	if ps.OwnsTask(id) {
		return ps.SetOwnedStatus(id, want)
	}
	// The cache only, which the next sync overwrites — what an agent HOLDS lives in agent_state, so
	// no non-owned task ever depended on this to be scheduled.
	t, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		return err
	}
	t.Status = want
	return ps.UpsertTask(t)
}

// settleWithTask rejects every pending PR against a just-closed task, naming the close as the
// reason. A rejection rather than a scrap: the work itself is not being judged and the branch keeps
// it, only the route in has gone. pr-sd-c9e829 drew two full reviews and a rejection asking for real
// work, every one of them after its task had closed.
func (e *Engine) settleWithTask(project, id string) {
	ps := e.store.For(project)
	prs, err := ps.PRs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hub: reading PRs after closing %s: %v\n", id, err)
		return
	}
	for _, pr := range prs {
		if pr.Task != id || !api.PROpen(pr) || pr.Status == "rejected" {
			continue
		}
		e.releaseReviewers(project, pr.ID, "its task closed")
		pr.Status, pr.Feedback = "rejected", "task "+id+" is closed"
		if perr := ps.PutPR(pr); perr != nil {
			fmt.Fprintf(os.Stderr, "hub: rejecting PR %s with its task: %v\n", pr.ID, perr)
			continue
		}
		_ = ps.LogPR(pr.ID, "rejected", "by hub: task "+id+" is closed")
		_ = e.hn.Say(project, pr.Agent, MsgPRSettledWithTask(pr.ID, id), MailAndPush)
	}
}

// finishTask is the shared close/scrap path: dispatch to the id's backend, then free whoever held
// the task. Cancelling mid-flight is allowed, never a refusal that strands the human.
func (e *Engine) finishTask(project, id string, scrap bool) error {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	// A parent is done exactly when its children are, so "done" is refused over open work — it would
	// hide those children from every view that walks the tree. The done close only: a scrap goes
	// deepest-first, and stranding subtasks as roots is its documented behaviour.
	if !scrap {
		open, err := ps.OpenChildIDs(id)
		if err != nil {
			return err
		}
		if len(open) > 0 {
			return fmt.Errorf("%s has open subtasks (%s) — close those first, or discard the whole tree with scrap --subtree",
				id, strings.Join(open, ", "))
		}
	}
	if err := e.finishAtSource(project, root, id, scrap); err != nil {
		return err
	}
	// A scrap settles its PRs through ScrapPR instead, which also discards the branch; doing it here
	// too would mark them terminal first and leave that branch standing.
	if !scrap {
		e.settleWithTask(project, id)
	}
	// Free whoever held it, so nobody grinds on dead work. The worktree is NOT reset here — the
	// agent may keep editing, so claimLeaf cleans up when it takes its next task.
	roster, _ := ps.Roster()
	for _, a := range roster {
		if st, _ := ps.GetState(a.Name); st.Task == id {
			// A container holder rests onto its FEATURE, not fully idle — dropping it here
			// silently unhooked the worker from the rest of what it still held (sd-5ef393).
			next := store.AgentState{Agent: a.Name, Phase: restPhase(a.Role)}
			if st.Container != "" {
				// Phase "idle" here, not restPhase(a.Role): only a worker holds a container today, so
				// the two agree — worth another look if a planner or coauthor ever comes to hold one.
				next = store.AgentState{Agent: a.Name, Container: st.Container, Branch: st.Container, Phase: "idle"}
			}
			verb := "closed"
			if scrap {
				verb = "scrapped"
			}
			_ = ps.SetState(next, store.ReasonFreed, "task "+verb+": "+id)
			_ = ps.Log(a.Name, "task-cancelled", id)
			// ESC first, so the cancellation lands on an idle prompt rather than queuing behind
			// the work it is cancelling. Only the interrupt needs the agent up; the delivery is
			// made either way, since mail is precisely what reaches one that is down.
			if e.hn.Observe(project, a.Name).Up {
				_ = e.hn.Interrupt(project, a.Name)
			}
			_ = e.hn.Say(project, a.Name, MsgTaskCancelled(id), MailAndPush)
		}
	}
	// The approval gate goes with the task: a gate left standing outlives what it asked about, and
	// the board reads it over the status — so a closed proposal kept rendering as "pending" and the
	// close looked as though it had not happened.
	if aerr := ps.ClearApproval(id); aerr != nil {
		fmt.Fprintf(os.Stderr, "hub: clearing the approval gate on %s: %v\n", id, aerr)
	}
	// Update the one row rather than re-syncing: SyncTasks refetches every source including a
	// GitHub call, so closing one task used to block for seconds on work it did not need.
	if scrap {
		if derr := ps.RemoveTask(id); derr != nil {
			fmt.Fprintf(os.Stderr, "hub: dropping scrapped task %s from cache: %v\n", id, derr)
		}
	} else if t, ok, gerr := ps.GetTask(id); gerr != nil {
		fmt.Fprintf(os.Stderr, "hub: reading task %s after close: %v\n", id, gerr)
	} else if ok {
		t.Status = "closed"
		t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if uerr := ps.UpsertTask(t); uerr != nil {
			fmt.Fprintf(os.Stderr, "hub: marking task %s closed in cache: %v\n", id, uerr)
		}
	}
	e.deps.Notify()
	return nil
}
