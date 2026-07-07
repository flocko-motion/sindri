// package: hub / workflow_merge
// type:    logic (merge workflow)
// job:     the human-gated merge of an approved PR into its base — rebase-first,
//          conflict routing to the worker, and the transient "merging" status plus
//          startup reconciliation to "merge-failed" for a merge orphaned by a crash.
// limits:  merge only; review/submit live in workflow_pr.go (same hub package).
package hub

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/hub/store"
)

// reconcileMergingPRs runs once at hub startup: any PR still in "merging" was being
// merged when the previous hub died, so its outcome is unknown (the base may or may
// not carry the merge). Move it to "merge-failed" — a visible state that asks for a
// human look — instead of leaving a silent, frozen "merging".
func (h *Hub) reconcileMergingPRs() {
	prs, err := h.store.AllPRs()
	if err != nil {
		log.Printf("hub: reconcile merging PRs: %v", err)
		return
	}
	for _, pr := range prs {
		if pr.Status != "merging" {
			continue
		}
		pr.Status = "merge-failed"
		ps := h.store.For(pr.Project)
		if err := ps.PutPR(pr); err != nil {
			log.Printf("hub: mark %s merge-failed: %v", pr.ID, err)
			continue
		}
		_ = ps.LogPR(pr.ID, "merge-failed", "hub restarted mid-merge — outcome unknown; inspect the base branch, then re-approve to retry")
		log.Printf("hub: %s was mid-merge at restart → merge-failed (inspect %s)", pr.ID, pr.Base)
	}
}

// Merge merges a project's approved PR into the base branch (host/human-only — the
// single hard gate), closes the task, frees the worker, and notifies it.
func (h *Hub) Merge(project, prID string) (store.PR, error) {
	ps := h.store.For(project)
	root := h.projectRoot(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return store.PR{}, err
	}
	if !ok {
		return store.PR{}, fmt.Errorf("no such PR %q", prID)
	}
	if pr.Status != "approved" {
		return store.PR{}, fmt.Errorf("%s is %s — only an approved PR may be merged", prID, pr.Status)
	}
	// Persist an explicit in-flight status: the merge is triggered but its outcome
	// isn't known yet. This keeps the board honest (a moving "merging", not a frozen
	// "approved") AND makes it recoverable — if the hub dies mid-merge, startup
	// reconciles any leftover "merging" to "merge-failed" (see reconcileMergingPRs),
	// because a half-applied merge needs a human look, not a silent retry.
	pr.Status = "merging"
	if err := ps.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	h.notify()
	// On a synchronous failure below, revert to approved so the PR stays retryable —
	// the returned error tells the human what to fix. (A conflict takes its own path
	// to "open"; a crash mid-merge is caught at startup, not here.)
	revert := func(e error) (store.PR, error) {
		pr.Status = "approved"
		_ = ps.PutPR(pr)
		h.notify()
		return store.PR{}, e
	}
	// Bring the branch up to the current base first. A merely-stale branch rebases
	// silently; a CONFLICT is routed into the worker's resolution loop — the hub
	// leaves the conflict in the worker's workspace to edit (it can't run git) and
	// the branch returns for review once clean, rather than a dead-end "resubmit".
	if a, ok, _ := ps.GetAgent(pr.Agent); ok {
		wt := filepath.Join(root, a.Workspace)
		conflicts, done, err := git.RebaseStart(wt, pr.Branch, pr.Base)
		if err != nil {
			// The rebase runs in the AGENT's worktree, not your checkout — say so, or
			// "unstaged changes" sends you hunting in the wrong place. Usually the agent
			// left uncommitted edits there; it must commit or discard them.
			return revert(fmt.Errorf("can't rebase %s onto %s in %s's worktree (%s) — most often it has uncommitted changes (NOT your checkout); have the agent commit or discard them, e.g. `sindri agent tell %s \"commit or discard your /workspace changes, then say done\"`. git said: %w",
				pr.Branch, pr.Base, pr.Agent, a.Workspace, pr.Agent, err))
		}
		if !done {
			pr.Status, pr.Feedback = "open", "" // no longer mergeable; back to review after the worker resolves
			_ = ps.PutPR(pr)
			_ = ps.SetState(store.AgentState{Agent: pr.Agent, Task: pr.Task, Branch: pr.Branch, Phase: "resolving"})
			_ = ps.LogPR(pr.ID, "conflict", "rebase onto "+pr.Base+" conflicts: "+strings.Join(conflicts, ", "))
			_ = h.injectWhenReady(project, pr.Agent, msgResolveNeeded(pr.Base, conflicts))
			h.notify()
			return store.PR{}, fmt.Errorf("%s conflicts with %s — sent to %s to resolve; it returns for review once clean", prID, pr.Base, pr.Agent)
		}
	}
	if err := git.Merge(root, pr.Base, pr.Branch); err != nil {
		if e := err.Error(); strings.Contains(e, "would be overwritten") || strings.Contains(e, "commit your changes or stash") {
			files := fileList(git.BlockingLocalChanges(root, pr.Base, pr.Branch))
			return revert(fmt.Errorf("merge blocked: commit or stash your local changes to %s in the working checkout, then merge again (the PR is fine and stays approved)", files))
		}
		return revert(err)
	}
	pr.Status = "merged"
	if err := ps.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	// Milestone PR for a held container: land the work but KEEP the branch/agent.
	if holder, _ := ps.GetState(pr.Agent); holder.Container != "" && holder.Container == pr.Branch {
		if a, ok, _ := ps.GetAgent(pr.Agent); ok {
			_ = git.RebaseOnto(filepath.Join(root, a.Workspace), pr.Branch, pr.Base) // ff past the merge
		}
		_ = ps.Log(pr.Agent, "merged", prID+" (milestone)")
		_ = ps.LogPR(prID, "merged", "milestone into "+pr.Base)
		h.resumeContainer(project, pr.Agent)
		_ = h.injectWhenReady(project, pr.Agent, msgMilestoneMerged(prID))
		h.rebasePlanners(project, pr.Base)
		h.notify()
		return pr, nil
	}
	if strings.HasPrefix(pr.Task, "td-") { // a planner's openspec PR has no real td task
		if err := td.Close(root, pr.Task, "merged via "+prID); err != nil {
			fmt.Printf("warning: td close %s: %v\n", pr.Task, err)
		}
		_ = h.refreshTask(project, pr.Task)
	}
	rest := "idle"
	if a, ok, _ := ps.GetAgent(pr.Agent); ok {
		rest = restPhase(a.Role)
	}
	_ = ps.SetState(store.AgentState{Agent: pr.Agent, Phase: rest})
	_ = ps.Log(pr.Agent, "merged", prID)
	_ = ps.LogPR(prID, "merged", "into "+pr.Base)
	_ = h.injectWhenReady(project, pr.Agent, msgMerged(prID))
	h.rebasePlanners(project, pr.Base) // any merge moves base → keep planners current
	h.notify()
	return pr, nil
}
