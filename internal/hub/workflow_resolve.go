// package: hub / workflow_resolve
// type:    logic (worker-driven merge-conflict resolution)
// job:     the `resolve` verb — bring a worker's submitted branch up to its base,
//          surfacing any conflict into the worker's workspace for it to edit while
//          the hub drives all git; once clean, renew the PR for review.
// limits:  git mechanics live in adapter/git; the merge gate lives in workflow_pr.
package hub

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// cmdResolve is the worker-driven mergeability loop: it brings the worker's
// submitted branch up to its base, and when that conflicts it leaves the conflict
// markers in the worker's workspace for it to edit, then continues on the next
// call. The worker may run it as often as it likes. All git runs host-side (the
// worker has none); once the branch applies cleanly the PR is renewed for review.
func (h *Hub) cmdResolve(c registry.Caller, _ []string, out io.Writer) (int, error) {
	ps := h.store.For(c.Project)
	root := h.projectRoot(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Branch == "" {
		fmt.Fprintln(out, "nothing to resolve — you have no submitted branch")
		return 1, nil
	}
	a, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, a.Workspace)
	base, err := h.baseBranch(root)
	if err != nil {
		return 1, err
	}
	var conflicts []string
	var done bool
	if git.RebaseInProgress(wt) { // a resolution already underway — advance it
		conflicts, done, err = git.RebaseContinue(wt)
	} else {
		conflicts, done, err = git.RebaseStart(wt, st.Branch, base)
	}
	if err != nil {
		return 1, err // internal git failure — AgentExec sanitizes it for the agent
	}
	keep := func(phase string) {
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: phase})
	}
	if !done {
		keep("resolving")
		_ = ps.Log(c.Agent, "resolve", "conflicts: "+strings.Join(conflicts, ", "))
		fmt.Fprintln(out, replyResolveConflicts(base, conflicts))
		return 0, nil
	}
	// Clean. If we were resolving a conflict, the branch changed — renew the PR so
	// the reviewer re-reviews before the human merges. A proactive resolve from a
	// still-current branch just reports "up to date".
	if st.Phase == "resolving" {
		if pr, ok, _ := ps.GetPR("pr-" + st.Task); ok {
			pr.Status, pr.Feedback = "open", ""
			_ = ps.PutPR(pr)
			_ = ps.LogPR(pr.ID, "renewed", "rebased clean onto "+base)
			h.notifyReviewers(c.Project, pr.ID, c.Agent)
		}
		fmt.Fprintln(out, replyResolvedClean(base))
	} else {
		fmt.Fprintln(out, replyAlreadyCurrent(base))
	}
	keep("submitted")
	h.notify()
	return 0, nil
}
