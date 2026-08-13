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
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/repo"
	"github.com/flo-at/sindri/internal/hub/store"
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
	active := map[string]map[string]string{}
	for _, pr := range prs {
		if !reg[pr.Project] {
			continue
		}
		if _, ok := active[pr.Project]; !ok {
			byPR, err := e.store.For(pr.Project).ActiveReviewers()
			if err != nil {
				return nil, err
			}
			active[pr.Project] = byPR
		}
		pr.Reviewer = active[pr.Project][pr.ID] // who is looking at it, for any list that shows PRs
		out = append(out, pr)
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

// PRDetail is a merge-intent plus its linked task and diff (for `pr info`). It
// crosses the wire, so it is internal/api.PRDetail under the name every existing
// caller here already uses.
type PRDetail = api.PRDetail

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
	if active, aerr := ps.ActiveReviewers(); aerr == nil {
		pr.Reviewer = active[id] // the same fact the lists carry, so the detail cannot disagree
	}
	diff, _ := git.Diff(e.deps.ProjectRoot(project), pr.Base, pr.Branch)
	task, _ := e.TaskInfo(project, pr.Task) // linked task; zero value if unreadable
	reviews, _ := ps.Reviews(id)
	lint, lintAt := ps.GetPRLint(id)
	history, _ := ps.PREvents(id)
	return PRDetail{PR: pr, Task: task, Diff: diff, Reviews: reviews, Lint: lint, LintAt: lintAt, History: history}, nil
}

// CmdSubmit returns immediately; the worker idles until the hub injects a verdict (D5).
func (e *Engine) CmdSubmit(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	root := e.deps.ProjectRoot(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	// What goes up: a whole feature branch when the worker holds one, otherwise the leaf task it is
	// working. A hierarchy changes the unit under review, never who puts it up — so the worker that
	// built it submits it, exactly as it would a task of its own.
	target, branch := st.Task, st.Branch
	if st.Container != "" {
		open, oerr := ps.OpenSubtasks(st.Container)
		if oerr != nil {
			return 1, oerr
		}
		if len(open) > 0 {
			fmt.Fprintln(out, ReplySubtasksRemain(st.Container, open[0].ID, len(open)))
			return 1, nil
		}
		target, branch = st.Container, st.Container
	} else if st.Phase != "working" || st.Task == "" {
		fmt.Fprintln(out, ReplyNotWorking("submit", st.Phase, st.Task))
		return 1, nil
	}
	a, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, a.Workspace)
	base, err := e.baseBranch(root)
	if err != nil {
		return 1, err
	}
	// Before the gate, not after: a branch that must rebase will be gated again on the rebased tree,
	// so running it now is a build and a test suite spent on a result nobody will keep.
	if refused, rerr := e.refuseIfBehind(ps, c.Agent, wt, base, target, out); rerr != nil || refused {
		return 1, rerr
	}
	if lintOut, ok := repo.Gate(wt, e.deps.BrokkrBin, e.verifyCmd(c.Project)); !ok {
		fmt.Fprintln(out, ReplyLintFail(strings.TrimSpace(lintOut)))
		_ = ps.Log(c.Agent, "lint-fail", target)
		return 1, nil
	}
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		msg = "work on " + target
	}
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	pr := store.PR{ID: "pr-" + target, Task: target, Agent: c.Agent, Branch: branch, Base: base, Status: "open"}
	_, existed, _ := ps.GetPR(pr.ID) // first submit vs a resubmit after rejection
	if err := ps.PutPR(pr); err != nil {
		return 1, err
	}
	if err := ps.SetState(store.AgentState{
		Agent: c.Agent, Task: st.Task, Branch: branch, Container: st.Container, Phase: "submitted",
	}); err != nil {
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

// refuseIfBehind stops a PR being recorded on a base the reference has moved past. It refuses rather
// than rebasing on the agent's behalf: the gate runs BEFORE the PR is written, so a silent rebase
// here would attach a gate result that never saw the merged state — passed against the old base,
// while the code that actually merges was never gated together. Sending the agent through `rebase`
// and a fresh submit re-runs the gate on the tree that will land.
//
// A failure to count is not a refusal. The count is the evidence, and blocking a submit on a git
// command that did not answer would strand an agent with finished work and nothing to fix.
func (e *Engine) refuseIfBehind(ps *store.ProjectStore, agent, wt, base, target string, out io.Writer) (refused bool, err error) {
	behind, cerr := git.CountRange(wt, "HEAD", base)
	if cerr != nil || behind == 0 {
		return false, nil
	}
	incoming, _ := git.LogRange(wt, "HEAD", base, logCap)
	fmt.Fprintln(out, ReplyBehindBase(base, behind, incoming))
	_ = ps.Log(agent, "submit-behind", fmt.Sprintf("%s: %d behind %s", target, behind, base))
	return true, nil
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
	if refused, rerr := e.refuseIfBehind(ps, c.Agent, wt, base, branch, out); rerr != nil || refused {
		return 1, rerr
	}
	// Gate on the installed quality gates (openspec validation, not the code linter: a planner may
	// only edit /workspace/openspec, so failing its plan on code it cannot touch would be wrong).
	if ok, valOut := e.qualityGate(wt); !ok {
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
	// The linked task, as the host's PR detail shows it. The hub already resolves it, so a reviewer
	// reading this saw the diff and never what it was for.
	if t, terr := e.TaskInfo(c.Project, pr.Task); terr == nil && t.ID != "" {
		fmt.Fprintf(out, "task:   %s  %s (%s)\n", t.ID, t.Title, t.Status)
	} else if pr.Task != "" {
		fmt.Fprintf(out, "task:   %s\n", pr.Task)
	}
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

// CmdRevoke withdraws the caller's own PR so it can keep working on the same branch. A worker that
// realises mid-review that something is missing had no way to say so: the only route out of
// "submitted" was somebody else's verdict, so it waited for a decision on work it already knew was
// incomplete — and since submit is the only thing that commits, whatever it wrote meanwhile was
// never recorded anywhere. This is a rejection the author issues, and it keeps the history.
func (e *Engine) CmdRevoke(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	pr, ok, err := e.livePR(c.Project, c.Agent)
	if err != nil {
		return 1, err
	}
	if !ok {
		fmt.Fprintln(out, ReplyNothingToRevoke)
		return 1, nil
	}
	reason := strings.TrimSpace(strings.Join(args, " "))
	if reason == "" {
		reason = "the author withdrew it"
	}
	pr.Status, pr.Feedback = "rejected", "withdrawn by "+c.Agent+": "+reason
	if err := ps.PutPR(pr); err != nil {
		return 1, err
	}
	// Back on the branch, exactly where submitting took it from — the container too, so a feature
	// worker returns to its own tree rather than falling out of the loop.
	if err := ps.SetState(store.AgentState{
		Agent: c.Agent, Task: st.Task, Branch: pr.Branch, Container: st.Container, Phase: "working",
	}); err != nil {
		return 1, err
	}
	// Whoever was reading it is reading a branch about to change under them.
	e.releaseReviewers(c.Project, pr.ID, "withdrawn by its author before a verdict")
	_ = ps.LogPR(pr.ID, "withdrawn", "by "+c.Agent+": "+reason)
	_ = ps.Log(c.Agent, "revoke", pr.ID+": "+reason)
	e.deps.Notify()
	fmt.Fprintln(out, ReplyRevoked(pr.ID, st.Task))
	return 0, nil
}

// livePR finds the PR an agent has out that has not landed or been discarded.
func (e *Engine) livePR(project, agent string) (store.PR, bool, error) {
	prs, err := e.store.For(project).PRs()
	if err != nil {
		return store.PR{}, false, err
	}
	for _, p := range prs {
		if p.Agent == agent && api.PROpen(p) {
			return p, true, nil
		}
	}
	return store.PR{}, false, nil
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
	// Only a LANDED or discarded PR refuses a verdict, which is api.PROpen's own line. An approved
	// one still takes a rejection: approval is the state before a merge, not a settled outcome, and
	// overruling a reviewer to stop something merging is the point of a human verdict. What must not
	// happen is a verdict on work already in the reference branch — writing one UNDID a merge in the
	// record, sent the author back to a landed branch, and looped the pair on an empty diff.
	if !api.PROpen(pr) {
		return fmt.Errorf("%s is %s — its work is already settled, so a verdict cannot change it", prID, pr.Status)
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
	// The held container is carried through the rejection: SetState writes the whole row, so leaving
	// it out dropped a feature worker out of the collaborative loop on a rejected milestone — it went
	// idle and claimed unrelated work, abandoning the feature branch its subtasks were on.
	prior, _ := ps.GetState(pr.Agent)
	_ = ps.SetState(store.AgentState{
		Agent: pr.Agent, Task: pr.Task, Branch: pr.Branch, Container: prior.Container, Phase: phase,
	})

	who, msg := "reviewer", MsgRejectedByReviewer(pr.ID, feedback)
	if byUser {
		who, msg = "user", MsgRejectedByUser(pr.ID, feedback)
	}
	if prior.Container != "" { // the milestone is the user's to re-open; there is nothing to re-submit
		msg = MsgMilestoneRejected(prior.Container, who, feedback)
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
	out, passed := repo.Gate(filepath.Join(e.deps.ProjectRoot(project), a.Workspace), e.deps.BrokkrBin, e.verifyCmd(project))
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
// It also settles the reference tip, since a merge moves it and the hub already handled that here —
// leaving it unrecorded would have SyncReference report the hub's own merge as an outside change.
func (e *Engine) rebasePlanners(project, base string) {
	defer e.noteReference(project)
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
	return e.openMilestone(project, agent, "")
}

// openMilestone puts a feature branch up as it stands: commit, record it as an interim PR the user
// merges, and keep the agent on the feature across the landing. One operation behind two doors — the
// human's milestone trigger and a worker's own `contribute` inside a feature — since partly landing a
// feature is the same act however it is asked for.
func (e *Engine) openMilestone(project, agent, msg string) (store.PR, error) {
	ps := e.store.For(project)
	root := e.deps.ProjectRoot(project)
	st, err := ps.GetState(agent)
	if err != nil {
		return store.PR{}, err
	}
	if st.Container == "" {
		return store.PR{}, fmt.Errorf("%s isn't working a feature — no milestone to open", agent)
	}
	a, ok, err := ps.GetAgent(agent)
	if err != nil || !ok {
		return store.PR{}, fmt.Errorf("no such agent %q", agent)
	}
	if msg == "" {
		msg = "milestone: " + st.Container
	}
	wt := filepath.Join(root, a.Workspace)
	if err := git.CommitAll(wt, msg); err != nil { // capture current state
		return store.PR{}, err
	}
	base, err := e.baseBranch(root)
	if err != nil {
		return store.PR{}, err
	}
	// Named for the FEATURE: the branch carries every checkpointed subtask, so naming it for the
	// subtask in hand would misdescribe what is in it. Interim, so nothing reads the merge as the
	// feature having landed — it is one instalment of a branch that goes on.
	pr := store.PR{ID: "pr-" + st.Container, Task: st.Container, Agent: agent, Branch: st.Container, Base: base, Status: "open", Kind: "interim"}
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
	_, ok, err := e.advanceContainer(project, agent, st.Container)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hub: advancing %s within %s: %v\n", agent, st.Container, err)
		return // leave the state as it is rather than parking it on a failure it can't see
	}
	if !ok {
		_ = ps.SetState(store.AgentState{Agent: agent, Container: st.Container, Branch: st.Container, Phase: "idle"})
		e.deps.Notify()
	}
}
