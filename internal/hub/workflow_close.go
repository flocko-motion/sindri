// package: hub / workflow_close
// type:    logic (task close/scrap dispatch)
// job:     the two lifecycle-ending verbs every todo backend distinguishes — "done"
//          (CloseTask) and "discard" (DeleteTask) — routed by the task's id prefix to
//          the right backend op: td close/delete, openspec archive/change-removal,
//          GitHub issue close/delete. Both guard against a live holder first.
// limits:  dispatch only; each op lives in its adapter (td/spec/github).
package hub

import (
	"context"
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/github"
	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/hub/store"
)

// CloseTask marks a task done from the task list (host-only) — the "done" close,
// dispatched by backend: a td task closes, an openspec change archives (its deltas
// fold into the specs), a GitHub issue is closed. Allowed even while an agent works
// on it — the agent is freed and told to pick up a new task (see finishTask).
func (h *Hub) CloseTask(project, id string) error { return h.finishTask(project, id, false) }

// DeleteTask scraps a task from the task list (host-only) — the "discard" close,
// dispatched by backend: a td task soft-deletes (restorable), an openspec change's
// proposal dir is removed (git-recoverable), a GitHub issue is deleted (permanent,
// needs repo-admin rights). Also allowed mid-flight (frees the holder).
func (h *Hub) DeleteTask(project, id string) error { return h.finishTask(project, id, true) }

// finishTask is the shared close/scrap path: it dispatches to the id's backend for
// either "done" (scrap=false) or "scrap" (scrap=true), then frees any agent that was
// working on the task and pushes it to pick up a new one — cancelling a task
// mid-flight is allowed, never a hard refusal that strands the human.
func (h *Hub) finishTask(project, id string, scrap bool) error {
	ps := h.store.For(project)
	root := h.projectRoot(project)
	var err error
	switch {
	case strings.HasPrefix(id, "td-"):
		if scrap {
			err = td.Delete(root, id)
		} else {
			err = td.Close(root, id, "closed from task list")
		}
	case strings.HasPrefix(id, "os-"):
		name, ok := h.changeName(root, id)
		if !ok {
			return fmt.Errorf("%s: can't resolve its openspec change (re-sync and retry)", id)
		}
		if scrap {
			err = spec.DeleteChange(root, name)
		} else {
			err = spec.Archive(root, name)
		}
	case strings.HasPrefix(id, "gh-"):
		n, ok := githubIssueNumber(id)
		if !ok {
			return fmt.Errorf("%s: not a valid GitHub id", id)
		}
		ctx, cancel := context.WithTimeout(context.Background(), githubIssueTimeout)
		defer cancel()
		if scrap {
			err = github.Delete(ctx, root, n)
		} else {
			err = github.Close(ctx, root, n, "closed via sindri")
		}
	default:
		return fmt.Errorf("%s: unknown task backend", id)
	}
	if err != nil {
		return err
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
			if h.agentAlive(project, a.Name) {
				_ = h.injectWhenReady(project, a.Name, msgTaskCancelled(id))
			}
		}
	}
	e := h.SyncTasks(project)
	h.notify()
	return e
}

// changeName resolves an os-<hash> id back to its openspec change name by matching
// specID over the current changes (the id is a one-way hash of the name).
func (h *Hub) changeName(root, id string) (string, bool) {
	changes, err := spec.Changes(root)
	if err != nil {
		return "", false
	}
	for _, c := range changes {
		if specID(c.Name) == id {
			return c.Name, true
		}
	}
	return "", false
}
