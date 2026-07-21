// package: hub/workflow / contribute
// type:    logic (mid-task contribution: interim PR)
// job:     CmdContribute — a worker lands work MID-task onto the reference branch
//          without finishing the task (for very large tasks). Commits, rebases onto
//          base to prove mergeability (conflicts hand off to resolve), records a gated
//          INTERIM PR; the user's merge keeps the task open and resumes the worker.
// limits:  no git here (-> adapter/git via hub/repo); persistence via the store.
package workflow

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
)

// CmdContribute lands an interim contribution: it commits the worktree, rebases the
// branch onto base (so the PR is immediately mergeable — a conflict drops the worker
// into the resolve loop, same as a merge conflict), and records a gated INTERIM PR the
// user approves like any other. Unlike submit it neither finishes the task nor spins a
// reviewer: on approval the merge keeps the task open and resumes the worker (see
// Merge's interim branch). The worker then waits (phase "submitted") until then.
func (e *Engine) CmdContribute(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	root := e.deps.ProjectRoot(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Phase != "working" || st.Task == "" {
		fmt.Fprintln(out, ReplyNothingToContribute)
		return 1, nil
	}
	a, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, a.Workspace)
	if lintOut, ok := repo.Lint(wt, e.deps.BrokkrBin); !ok {
		fmt.Fprintln(out, ReplyLintFail(strings.TrimSpace(lintOut)))
		_ = ps.Log(c.Agent, "lint-fail", st.Task)
		return 1, nil
	}
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		msg = "interim contribution on " + st.Task
	}
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	base, err := e.baseBranch(root)
	if err != nil {
		return 1, err
	}

	// Record the interim intent up front so a conflict can hand off to the shared
	// resolve loop (which renews an existing PR when the branch comes clean) — the same
	// path a final PR's merge conflict uses. Gated: no reviewer is requested; the user
	// approves it.
	pr := store.PR{ID: "pr-" + st.Task, Task: st.Task, Agent: c.Agent, Branch: st.Branch, Base: base, Status: "open", Kind: "interim"}
	_, existed, _ := ps.GetPR(pr.ID)
	if err := ps.PutPR(pr); err != nil {
		return 1, err
	}

	// Rebase onto base to prove the contribution merges. A conflict is left in the
	// worktree with markers; the worker resolves it via `sindri resolve`, which renews
	// this PR when clean (no reviewer, since it's interim).
	conflicts, done, err := repo.RebaseStep(wt, st.Branch, base)
	if err != nil {
		return 1, err
	}
	if !done {
		_ = ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "resolving"})
		_ = ps.Log(c.Agent, "contribute-conflict", strings.Join(conflicts, ", "))
		fmt.Fprintln(out, ReplyContributeConflicts(base, conflicts))
		return 0, nil
	}

	if err := ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Phase: "submitted"}); err != nil {
		return 1, err
	}
	_ = ps.Log(c.Agent, "contribute", pr.ID)
	if existed {
		_ = ps.LogPR(pr.ID, "resubmitted", "interim, by "+c.Agent+": "+msg)
	} else {
		_ = ps.LogPR(pr.ID, "created", "interim, by "+c.Agent+": "+msg)
	}
	e.deps.Notify()
	fmt.Fprintln(out, ReplyContributed(pr.ID))
	return 0, nil
}
