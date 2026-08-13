// package: hub/workflow / contribute
// type:    logic (mid-task contribution: interim PR)
// job:     CmdContribute — a worker lands work MID-task, without finishing it.
// Commits, rebases onto base to prove mergeability (conflicts hand off to
// resolve), records a gated INTERIM PR the user's merge keeps the task open
// on. Inside a feature the whole branch goes up (-> pr.go openMilestone).
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

// CmdContribute lands an interim contribution: commit, rebase onto base so the PR is immediately
// mergeable, and record a user-gated INTERIM PR. Unlike submit it neither finishes the task nor
// spins a reviewer — on approval the merge keeps the task open and resumes the worker. A worker
// inside a feature contributes the whole branch as a milestone, where the wait for others to see
// the work is longest.
func (e *Engine) CmdContribute(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	root := e.deps.ProjectRoot(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	// Inside a feature what is worth landing is the branch, not the subtask in hand — which is the
	// operation the user's milestone trigger already performs, so this is the same door for the agent.
	if st.Container == "" && (st.Phase != "working" || st.Task == "") {
		fmt.Fprintln(out, ReplyNotWorking("contribute", st.Phase, st.Task))
		return 1, nil
	}
	a, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, a.Workspace)
	if lintOut, ok := repo.Gate(wt, e.deps.BrokkrBin, e.verifyCmd(c.Project)); !ok {
		fmt.Fprintln(out, ReplyLintFail(strings.TrimSpace(lintOut)))
		_ = ps.Log(c.Agent, "lint-fail", st.Task)
		return 1, nil
	}
	msg := strings.TrimSpace(strings.Join(args, " "))
	if st.Container != "" {
		pr, err := e.openMilestone(c.Project, c.Agent, msg)
		if err != nil {
			return 1, err
		}
		fmt.Fprintln(out, ReplyMilestoneContributed(pr.ID, st.Container))
		return 0, nil
	}
	tk, _, _ := ps.GetTask(st.Task)
	if msg == "" {
		msg = tk.Title
	}
	if msg == "" {
		msg = "interim contribution on " + st.Task
	}
	msg = conventionalCommit(tk.Type, st.Task, msg)
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

	if err := ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "submitted"}); err != nil {
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
