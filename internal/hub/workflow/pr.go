// package: hub/workflow / pr
// type:    logic (PR-as-merge-intent: submit → review → approve → host merge)
// job:     the reviewer verbs and the host merge; verdicts route to the owning
// agent's session by branch (object-mediated, D-routing). git is hub-side.
// All state is per-project — methods take a project (repoTag) and work
// through store.For(project) + e.deps.ProjectRoot(project).
// limits:  the PR side only; task claim/submit-to-td is workflow_task.go and the
// git mechanics are the adapter's (-> adapter/git).
package workflow

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/tasks/spec"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// baseBranch is the branch agents work against: the configured `reference:`, else the main
// checkout's current branch. Configured-but-absent is fatal — substituting one would corrupt every
// claim, submit and merge measured against it.
func (e *Engine) baseBranch(root string) (string, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return "", err
	}
	if cfg.Reference == "" {
		return git.CurrentBranch(root)
	}
	if !git.BranchExists(root, cfg.Reference) {
		return "", fmt.Errorf("the configured reference branch %q doesn't exist in %s — create it or fix `reference:` in .sindri/config.yaml", cfg.Reference, root)
	}
	return cfg.Reference, nil
}

// FleetPRs is fleet-wide, so `pr list` matches the TUI regardless of the caller's cwd.
func (e *Engine) FleetPRs() ([]store.PR, error) {
	prs, err := e.store.AllPRs()
	if err != nil {
		return nil, err
	}
	reg := map[string]bool{}
	for _, p := range e.deps.KnownProjects() {
		reg[p.Tag] = true
	}
	out := make([]store.PR, 0, len(prs))
	for _, pr := range prs {
		if reg[pr.Project] {
			out = append(out, pr)
		}
	}
	return out, nil
}

// PRProject finds a PR's owner by id, because a cwd-derived project differs between a
// worktree subdir and the repo root. The caller's own project wins any id clash.
func (e *Engine) PRProject(fallback, id string) string {
	if _, ok, _ := e.store.For(fallback).GetPR(id); ok {
		return fallback
	}
	if prs, err := e.store.AllPRs(); err == nil {
		for _, p := range prs {
			if p.ID == id {
				return p.Project
			}
		}
	}
	return fallback
}

// PRDetail is a merge-intent plus its linked task and diff (for `pr info`).
type PRDetail struct {
	PR      store.PR       `json:"pr"`
	Task    store.Task     `json:"task"`
	Diff    string         `json:"diff"`
	Reviews []store.Review `json:"reviews"`
	Lint    string         `json:"lint"`    // latest stored lint output ("" = never run)
	LintAt  string         `json:"lint_at"` // when it was run
	History []store.Event  `json:"history"` // lifecycle log (oldest-first)
}

// PRInfo returns a project's PR with its linked task and diff.
func (e *Engine) PRInfo(project, id string) (PRDetail, error) {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(id)
	if err != nil {
		return PRDetail{}, err
	}
	if !ok {
		return PRDetail{}, fmt.Errorf("no such PR %q", id)
	}
	diff, _ := git.Diff(e.deps.ProjectRoot(project), pr.Base, pr.Branch)
	task, _ := e.TaskInfo(project, pr.Task) // linked task; zero value if unreadable
	reviews, _ := ps.Reviews(id)
	lint, lintAt := ps.GetPRLint(id)
	history, _ := ps.PREvents(id)
	return PRDetail{PR: pr, Task: task, Diff: diff, Reviews: reviews, Lint: lint, LintAt: lintAt, History: history}, nil
}

// ReviewPrompt reads review-prompt.txt, auto-created from a built-in default if absent.
func (e *Engine) ReviewPrompt(project string) (string, error) {
	// A repo-committed `review_prompt` wins; config already validated the path exists.
	if cfg, err := e.deps.ProjectConfig(project); err != nil {
		return "", err
	} else if cfg.ReviewPrompt != "" {
		data, rerr := os.ReadFile(config.Abs(e.deps.ProjectRoot(project), cfg.ReviewPrompt))
		if rerr != nil {
			return "", fmt.Errorf("read review_prompt %s: %w", cfg.ReviewPrompt, rerr)
		}
		return strings.TrimSpace(string(data)), nil
	}
	dir := filepath.Join(paths.StateDir(), project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "review-prompt.txt")
	if data, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(data)), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(path, []byte(DefaultReviewPrompt+"\n"), 0o644); err != nil {
		return "", err
	}
	return DefaultReviewPrompt, nil
}

// RequestReview is the ONE review path: every trigger funnels here, so a review is always
// the same thing. No reviewer running → recorded unassigned; requirement "" uses the default.
func (e *Engine) RequestReview(project, prID, requirement string) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}
	requirement = strings.TrimSpace(requirement)
	if requirement == "" {
		requirement, _ = e.ReviewPrompt(project)
	}
	id, err := ps.AddReview(prID, requirement)
	if err != nil {
		return err
	}
	reviewer, err := e.runningReviewer(project)
	if err != nil {
		return err
	}
	if reviewer == "" {
		_ = ps.LogPR(prID, "review-requested", "unassigned (no reviewer running)")
		e.deps.Notify()
		return nil
	}
	if err := ps.AssignReview(id, reviewer); err != nil {
		return err
	}
	// The hub preps the terrain so the reviewer never faces a stale tree: force-checkout is
	// safe because it only reads + lints. On failure it is told not to trust /workspace.
	checkedOut := true
	if a, ok, gerr := ps.GetAgent(reviewer); gerr != nil || !ok {
		checkedOut = false
		_ = ps.LogPR(prID, "checkout-failed", "reviewer "+reviewer+" not on roster")
	} else if coErr := git.CheckoutDetachedClean(filepath.Join(e.deps.ProjectRoot(project), a.Workspace), pr.Branch); coErr != nil {
		checkedOut = false
		_ = ps.LogPR(prID, "checkout-failed", fmt.Sprintf("%s into %s: %v", pr.Branch, a.Workspace, coErr))
	}
	_ = ps.SetState(store.AgentState{Agent: reviewer, Phase: "reviewing"}) // board shows it working, not idle
	_ = ps.LogPR(prID, "review-requested", "assigned to "+reviewer)
	go e.deps.InjectWhenReady(project, reviewer, MsgReview(prID, requirement, pr.Branch, pr.Base, e.deps.ArchitectureDoc(project), checkedOut)) // async: don't block a worker's submit
	e.deps.Notify()
	return nil
}

// runningReviewer returns a roster read failure rather than disguising it as "no reviewer",
// which would silently drop the review request.
func (e *Engine) runningReviewer(project string) (string, error) {
	roster, err := e.store.For(project).Roster()
	if err != nil {
		return "", fmt.Errorf("load roster for %s: %w", project, err)
	}
	for _, a := range roster {
		if a.Role == "reviewer" && container.Running(e.deps.Container(project, a.Name)) && e.deps.SessionAlive(project, a.Name) {
			return a.Name, nil
		}
	}
	return "", nil
}

// CmdSubmit returns immediately; the worker idles until the hub injects a verdict (D5).
func (e *Engine) CmdSubmit(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	root := e.deps.ProjectRoot(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Phase != "working" || st.Task == "" {
		fmt.Fprintln(out, ReplyNotWorking("submit", st.Phase, st.Task))
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
		msg = "work on " + st.Task
	}
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	base, err := e.baseBranch(root)
	if err != nil {
		return 1, err
	}
	pr := store.PR{ID: "pr-" + st.Task, Task: st.Task, Agent: c.Agent, Branch: st.Branch, Base: base, Status: "open"}
	_, existed, _ := ps.GetPR(pr.ID) // first submit vs a resubmit after rejection
	if err := ps.PutPR(pr); err != nil {
		return 1, err
	}
	if err := ps.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Phase: "submitted"}); err != nil {
		return 1, err
	}
	_ = ps.Log(c.Agent, "submit", pr.ID)
	if existed {
		_ = ps.LogPR(pr.ID, "resubmitted", "by "+c.Agent+": "+msg)
	} else {
		_ = ps.LogPR(pr.ID, "created", "by "+c.Agent+": "+msg)
	}
	_ = e.RequestReview(c.Project, pr.ID, "") // one review path; the hub preps the terrain
	fmt.Fprintln(out, ReplyRegistered(pr.ID))
	return 0, nil
}

// CmdOpenspec is the planner's ship verb: openspec edits become a PR on its standing branch.
func (e *Engine) CmdOpenspec(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 || args[0] != "submit" {
		fmt.Fprintln(out, "usage: openspec submit [message]")
		return 2, nil
	}
	ps := e.store.For(c.Project)
	root := e.deps.ProjectRoot(c.Project)
	a, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, a.Workspace)
	base, err := e.baseBranch(root)
	if err != nil {
		return 1, err
	}
	branch := PlannerBranch(c.Agent)
	changed, err := git.HasChanges(wt)
	if err != nil {
		return 1, err
	}
	ahead, err := git.Ahead(wt, base)
	if err != nil {
		return 1, err
	}
	if !changed && !ahead {
		fmt.Fprintln(out, "Nothing to submit — edit /workspace/openspec first.")
		return 1, nil
	}
	// Gate on openspec VALIDATION, not the code linter: a planner may only edit
	// /workspace/openspec, so failing its plan on code it cannot touch would be wrong.
	if ok, valOut := spec.Validate(wt); !ok {
		fmt.Fprintln(out, ReplySpecInvalid(strings.TrimSpace(valOut)))
		_ = ps.Log(c.Agent, "openspec-invalid", branch)
		return 1, nil
	}
	msg := strings.TrimSpace(strings.Join(args[1:], " "))
	if msg == "" {
		msg = "openspec update"
	}
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	pr := store.PR{ID: "pr-" + branch, Task: mockSpecTask, Agent: c.Agent, Branch: branch, Base: base, Status: "open"}
	_, existed, _ := ps.GetPR(pr.ID)
	if err := ps.PutPR(pr); err != nil {
		return 1, err
	}
	if err := ps.SetState(store.AgentState{Agent: c.Agent, Task: mockSpecTask, Branch: branch, Phase: "submitted"}); err != nil {
		return 1, err
	}
	_ = ps.Log(c.Agent, "submit", pr.ID)
	if existed {
		_ = ps.LogPR(pr.ID, "resubmitted", "by "+c.Agent+": "+msg)
	} else {
		_ = ps.LogPR(pr.ID, "created", "by "+c.Agent+": "+msg)
	}
	_ = e.RequestReview(c.Project, pr.ID, "") // one review path; the hub preps the terrain
	fmt.Fprintln(out, ReplyRegistered(pr.ID))
	return 0, nil
}

// CmdShowPR prints a PR's metadata and diff so a reviewer can judge it.
func (e *Engine) CmdShowPR(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: show <pr-id>")
		return 2, nil
	}
	pr, ok, err := e.store.For(c.Project).GetPR(args[0])
	if err != nil {
		return 1, err
	}
	if !ok {
		return 1, fmt.Errorf("no such PR %q", args[0])
	}
	fmt.Fprintf(out, "%s  [%s]  by %s\nbranch %s → %s\n", pr.ID, pr.Status, pr.Agent, pr.Branch, pr.Base)
	if pr.Feedback != "" {
		fmt.Fprintf(out, "feedback: %s\n", pr.Feedback)
	}
	diff, err := git.Diff(e.deps.ProjectRoot(c.Project), pr.Base, pr.Branch)
	if err != nil {
		return 1, err
	}
	fmt.Fprintf(out, "\n%s\n", strings.TrimSpace(diff))
	return 0, nil
}

// openPR takes an explicit id, else the oldest open PR.
func (e *Engine) openPR(project string, args []string) (store.PR, error) {
	ps := e.store.For(project)
	if len(args) > 0 {
		pr, ok, err := ps.GetPR(args[0])
		if err != nil {
			return store.PR{}, err
		}
		if !ok {
			return store.PR{}, fmt.Errorf("no such PR %q", args[0])
		}
		return pr, nil
	}
	open, err := ps.PRs("open")
	if err != nil {
		return store.PR{}, err
	}
	if len(open) == 0 {
		return store.PR{}, fmt.Errorf("no open PRs")
	}
	return open[len(open)-1], nil // oldest
}

// CmdApprove marks a PR approved (the human still merges — the only hard gate).
func (e *Engine) CmdApprove(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	pr, err := e.openPR(c.Project, args)
	if err != nil {
		return 1, err
	}
	if pr.Status != "open" {
		fmt.Fprintf(out, "%s is %s — only an open PR can be approved.\n", pr.ID, pr.Status)
		return 1, nil
	}
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		return 1, err
	}
	_ = ps.Log(c.Agent, "approve", pr.ID)
	_ = ps.LogPR(pr.ID, "approved", "by "+c.Agent)
	e.completeReview(c.Project, pr.ID, c.Agent, "pass", "") // record the verdict + return the reviewer to idle
	e.deps.Notify()
	fmt.Fprintf(out, "%s approved — awaiting human merge ('sindri merge %s').\n", pr.ID, pr.ID)
	return 0, nil
}

// completeReview stamps the verdict (a human verdict has no record) and returns the
// reviewer to idle, so a finished review stops showing as "reviewing".
func (e *Engine) completeReview(project, prID, agent, verdict, findings string) {
	ps := e.store.For(project)
	if revs, err := ps.Reviews(prID); err == nil {
		for _, r := range revs {
			if r.Author == agent && r.Verdict == "" {
				_ = ps.RecordVerdict(r.ID, verdict, findings)
				break
			}
		}
	}
	_ = ps.SetState(store.AgentState{Agent: agent, Phase: "idle"})
}

// ApprovePR is the human approve path (TUI/CLI): marks a project's open PR approved.
func (e *Engine) ApprovePR(project, prID string) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}
	if pr.Status != "open" {
		return fmt.Errorf("%s is %s — only an open PR can be approved", prID, pr.Status)
	}
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		return err
	}
	_ = ps.LogPR(prID, "approved", "by user")
	e.deps.Notify()
	return nil
}

// RejectPR is the human reject path: the owning worker resubmits, told in the [user] voice.
func (e *Engine) RejectPR(project, prID, feedback string) error {
	return e.reject(project, prID, feedback, true)
}

// reject routes feedback to the owning worker; byUser picks the [user]/[reviewer] voice.
func (e *Engine) reject(project, prID, feedback string, byUser bool) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		feedback = "changes requested"
	}
	pr.Status, pr.Feedback = "rejected", feedback
	if err := ps.PutPR(pr); err != nil {
		return err
	}
	phase := "working"
	if a, ok, _ := ps.GetAgent(pr.Agent); ok && a.Role == "planner" {
		phase = restPhase(a.Role)
	}
	_ = ps.SetState(store.AgentState{Agent: pr.Agent, Task: pr.Task, Branch: pr.Branch, Phase: phase})

	who, msg := "reviewer", MsgRejectedByReviewer(pr.ID, feedback)
	if byUser {
		who, msg = "user", MsgRejectedByUser(pr.ID, feedback)
	}
	_ = ps.LogPR(pr.ID, "rejected", "by "+who+": "+feedback)
	_ = ps.Log(pr.Agent, "reject", pr.ID+" ("+who+"): "+feedback)
	_ = e.deps.InjectWhenReady(project, pr.Agent, msg)
	e.deps.Notify()
	return nil
}

// MaterializeReview detaches the PR branch into .worktrees/review for a human to inspect.
func (e *Engine) MaterializeReview(project, prID string) (string, error) {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no such PR %q", prID)
	}
	return repo.MaterializeReview(root, pr.Branch)
}

// LintPR runs the quality gate on a PR worktree, headed with PASS/FAIL.
func (e *Engine) LintPR(project, prID string) (string, error) {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no such PR %q", prID)
	}
	a, ok, err := ps.GetAgent(pr.Agent)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no agent %q for %s", pr.Agent, prID)
	}
	out, passed := repo.Lint(filepath.Join(e.deps.ProjectRoot(project), a.Workspace), e.deps.BrokkrBin)
	status := "FAIL"
	if passed {
		status = "PASS"
	}
	if strings.TrimSpace(out) == "" {
		out = "(no output)\n"
	}
	result := fmt.Sprintf("lint %s\n\n%s", status, out)
	_ = ps.SetPRLint(prID, result) // persist the latest result
	return result, nil
}

// CmdReject is the agent-reviewer reject: [reviewer] voice, "changes" verdict, back to idle.
func (e *Engine) CmdReject(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: reject <pr-id> <feedback...>")
		return 2, nil
	}
	feedback := strings.Join(args[1:], " ")
	if err := e.reject(c.Project, args[0], feedback, false); err != nil {
		return 1, err
	}
	e.completeReview(c.Project, args[0], c.Agent, "changes", strings.TrimSpace(feedback))
	fmt.Fprintf(out, "%s rejected; worker notified.\n", args[0])
	return 0, nil
}

// RebaseAgent recovers a stale tree after the base moved outside a sindri merge; git aborts
// on conflict, so nothing changes. A coauthor shares the user's checkout, so it is refused.
func (e *Engine) RebaseAgent(project, name string) error {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	if a.Workspace == "." {
		return fmt.Errorf("%s is a coauthor sharing your working checkout — rebase that yourself with git, not through sindri", name)
	}
	base, err := e.baseBranch(root)
	if err != nil {
		return err
	}
	if err := git.Rebase(filepath.Join(root, a.Workspace), base); err != nil {
		return fmt.Errorf("couldn't rebase %s onto %s — a conflict or uncommitted changes (git aborted, so nothing changed). Have %s resolve it interactively with `sindri rebase` (it surfaces the conflicts to fix). git said: %w", name, base, name, err)
	}
	_ = ps.Log(name, "rebase", "onto "+base)
	_ = e.deps.InjectWhenReady(project, name, MsgRebased(base))
	e.deps.Notify()
	return nil
}

// rebasePlanners is best-effort after a merge: a dirty or conflicting worktree is logged, skipped.
func (e *Engine) rebasePlanners(project, base string) {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	roster, _ := ps.Roster()
	for _, a := range roster {
		if a.Role != "planner" {
			continue
		}
		wt := filepath.Join(root, a.Workspace)
		if err := git.Rebase(wt, base); err != nil {
			_ = ps.Log(a.Name, "rebase-skip", base+": "+err.Error())
			continue
		}
		_ = ps.Log(a.Name, "rebase", "onto "+base)
		_ = e.deps.InjectWhenReady(project, a.Name, MsgRebased(base))
	}
}

// MilestonePR opens (or refreshes) a container milestone, blocking the agent until a human merges.
func (e *Engine) MilestonePR(project, agent string) (store.PR, error) {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	st, err := ps.GetState(agent)
	if err != nil {
		return store.PR{}, err
	}
	if st.Container == "" {
		return store.PR{}, fmt.Errorf("%s isn't working a container — no milestone to open", agent)
	}
	a, ok, err := ps.GetAgent(agent)
	if err != nil || !ok {
		return store.PR{}, fmt.Errorf("no such agent %q", agent)
	}
	wt := filepath.Join(root, a.Workspace)
	if err := git.CommitAll(wt, "milestone: "+st.Container); err != nil { // capture current state
		return store.PR{}, err
	}
	base, err := e.baseBranch(root)
	if err != nil {
		return store.PR{}, err
	}
	pr := store.PR{ID: "pr-" + st.Container, Task: st.Container, Agent: agent, Branch: st.Container, Base: base, Status: "open"}
	_, existed, _ := ps.GetPR(pr.ID)
	if err := ps.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	if err := ps.SetState(store.AgentState{Agent: agent, Container: st.Container, Branch: st.Container, Task: st.Task, Phase: "submitted"}); err != nil {
		return store.PR{}, err
	}
	if existed {
		_ = ps.LogPR(pr.ID, "resubmitted", "milestone by "+agent)
	} else {
		_ = ps.LogPR(pr.ID, "created", "milestone by "+agent)
	}
	_ = ps.Log(agent, "milestone", pr.ID)
	e.deps.Notify()
	return pr, nil
}

// resumeContainer puts a container's agent back to work after a milestone merge.
func (e *Engine) resumeContainer(project, agent string) {
	ps := e.store.For(project)
	st, _ := ps.GetState(agent)
	if st.Container == "" {
		return
	}
	if st.Task != "" {
		if t, ok, _ := ps.GetTask(st.Task); ok && (t.Status == "open" || t.Status == "in_progress") {
			_ = ps.SetState(store.AgentState{Agent: agent, Container: st.Container, Branch: st.Container, Task: st.Task, Phase: "working"})
			e.deps.Notify()
			return
		}
	}
	if _, ok := e.advanceContainer(project, agent, st.Container); !ok {
		_ = ps.SetState(store.AgentState{Agent: agent, Container: st.Container, Branch: st.Container, Phase: "idle"})
		e.deps.Notify()
	}
}
