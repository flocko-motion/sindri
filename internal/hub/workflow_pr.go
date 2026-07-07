// package: hub / workflow (PR, review, merge)
// type:    logic (PR-as-merge-intent: submit → review → approve → host merge)
// job:     the reviewer verbs and the host merge; verdicts route to the owning
//          agent's session by branch (object-mediated, D-routing). git is hub-side.
//          All state is per-project — methods take a project (repoTag) and work
//          through store.For(project) + h.projectRoot(project).
// limits:  the PR side only; task claim/submit-to-td is workflow_task.go and the
//          git mechanics are the adapter's (-> adapter/git).
package hub

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/paths"
)

// baseBranch reads a repo's base branch from its main checkout.
func (h *Hub) baseBranch(root string) (string, error) { return git.CurrentBranch(root) }

// PRs returns a project's merge-intents (newest first).
func (h *Hub) PRs(project string) ([]store.PR, error) { return h.store.For(project).PRs() }

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
func (h *Hub) PRInfo(project, id string) (PRDetail, error) {
	ps := h.store.For(project)
	pr, ok, err := ps.GetPR(id)
	if err != nil {
		return PRDetail{}, err
	}
	if !ok {
		return PRDetail{}, fmt.Errorf("no such PR %q", id)
	}
	diff, _ := git.Diff(h.projectRoot(project), pr.Base, pr.Branch)
	task, _ := h.TaskInfo(project, pr.Task) // linked task; zero value if unreadable
	reviews, _ := ps.Reviews(id)
	lint, lintAt := ps.GetPRLint(id)
	history, _ := ps.PREvents(id)
	return PRDetail{PR: pr, Task: task, Diff: diff, Reviews: reviews, Lint: lint, LintAt: lintAt, History: history}, nil
}

// ReviewPrompt returns a project's default agentic-review instruction, read from
// its central review-prompt.txt — auto-created with a built-in default if absent.
func (h *Hub) ReviewPrompt(project string) (string, error) {
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
	if err := os.WriteFile(path, []byte(defaultReviewPrompt+"\n"), 0o644); err != nil {
		return "", err
	}
	return defaultReviewPrompt, nil
}

// RequestReview attaches a review requirement to a project's PR and dispatches it
// to a running reviewer (or records it unassigned for one to pick up later).
func (h *Hub) RequestReview(project, prID, requirement string) error {
	ps := h.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}
	requirement = strings.TrimSpace(requirement)
	if requirement == "" {
		requirement, _ = h.ReviewPrompt(project)
	}
	id, err := ps.AddReview(prID, requirement)
	if err != nil {
		return err
	}
	reviewer, err := h.runningReviewer(project)
	if err != nil {
		return err
	}
	if reviewer != "" {
		if err := ps.AssignReview(id, reviewer); err != nil {
			return err
		}
		_ = ps.LogPR(prID, "review-requested", "assigned to "+reviewer)
		_ = h.assignedReviewInject(project, reviewer, pr, prID, requirement)
		h.notify()
		return nil
	}
	_ = ps.LogPR(prID, "review-requested", "unassigned (no reviewer running)")
	h.notify()
	return nil
}

// assignedReviewInject checks the PR branch out into the reviewer's workspace and
// injects the review instruction. A checkout failure is loud (recorded), never silent.
func (h *Hub) assignedReviewInject(project, reviewer string, pr store.PR, prID, requirement string) error {
	ps := h.store.For(project)
	checkedOut := false
	a, ok, err := ps.GetAgent(reviewer)
	switch {
	case err != nil:
		_ = ps.LogPR(prID, "checkout-failed", fmt.Sprintf("reviewer %s lookup: %v", reviewer, err))
	case !ok:
		_ = ps.LogPR(prID, "checkout-failed", "reviewer "+reviewer+" has no roster entry")
	default:
		if coErr := git.CheckoutDetached(filepath.Join(h.projectRoot(project), a.Workspace), pr.Branch); coErr != nil {
			_ = ps.LogPR(prID, "checkout-failed", fmt.Sprintf("%s into %s: %v", pr.Branch, a.Workspace, coErr))
		} else {
			checkedOut = true
		}
	}
	return h.injectWhenReady(project, reviewer, msgReviewAssigned(prID, requirement, pr.Branch, pr.Base, checkedOut))
}

// runningReviewer returns the name of a live reviewer agent in a project, or "". A
// roster read failure is returned, not disguised as "no reviewer running" (which
// would silently drop the review request).
func (h *Hub) runningReviewer(project string) (string, error) {
	roster, err := h.store.For(project).Roster()
	if err != nil {
		return "", fmt.Errorf("load roster for %s: %w", project, err)
	}
	for _, a := range roster {
		if a.Role == "reviewer" && container.Running(h.container(project, a.Name)) && h.sessionAlive(project, a.Name) {
			return a.Name, nil
		}
	}
	return "", nil
}

// runLint runs the project's quality gates against a worktree by invoking `brokkr
// lint` there (a subprocess, so the concurrent hub never chdir's). Go modules only.
func (h *Hub) runLint(wt string) (output string, ok bool) {
	if _, err := os.Stat(filepath.Join(wt, "go.mod")); err != nil {
		return "", true // no Go module — no lint gate applies
	}
	bin, err := brokkrBinary()
	if err != nil {
		return "lint: " + err.Error(), false
	}
	cmd := exec.Command(bin, "lint")
	cmd.Dir = wt
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// cmdSubmit commits the worker's worktree, records a merge-intent, and returns
// immediately — the worker then goes idle until the hub injects a verdict (D5).
func (h *Hub) cmdSubmit(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := h.store.For(c.Project)
	root := h.projectRoot(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Phase != "working" || st.Task == "" {
		fmt.Fprintln(out, replyNothingToSubmit)
		return 1, nil
	}
	a, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, a.Workspace)
	if lintOut, ok := h.runLint(wt); !ok {
		fmt.Fprintln(out, replyLintFail(strings.TrimSpace(lintOut)))
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
	base, err := h.baseBranch(root)
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
	h.notifyReviewers(c.Project, pr.ID, c.Agent)
	fmt.Fprintln(out, replyRegistered(pr.ID))
	return 0, nil
}

// cmdOpenspec is the planner's ship verb: turns its openspec edits into a PR on its
// standing branch (mock todo id os-new).
func (h *Hub) cmdOpenspec(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 || args[0] != "submit" {
		return 1, fmt.Errorf("usage: openspec submit [message]")
	}
	ps := h.store.For(c.Project)
	root := h.projectRoot(c.Project)
	a, _, _ := ps.GetAgent(c.Agent)
	wt := filepath.Join(root, a.Workspace)
	base, err := h.baseBranch(root)
	if err != nil {
		return 1, err
	}
	branch := plannerBranch(c.Agent)
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
	if lintOut, ok := h.runLint(wt); !ok {
		fmt.Fprintln(out, replyLintFail(strings.TrimSpace(lintOut)))
		_ = ps.Log(c.Agent, "lint-fail", branch)
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
	h.notifyReviewers(c.Project, pr.ID, c.Agent)
	fmt.Fprintln(out, replyRegistered(pr.ID))
	return 0, nil
}

// cmdShowPR prints a PR's metadata and diff so a reviewer can judge it.
func (h *Hub) cmdShowPR(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		return 1, fmt.Errorf("usage: show <pr-id>")
	}
	pr, ok, err := h.store.For(c.Project).GetPR(args[0])
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
	diff, err := git.Diff(h.projectRoot(c.Project), pr.Base, pr.Branch)
	if err != nil {
		return 1, err
	}
	fmt.Fprintf(out, "\n%s\n", strings.TrimSpace(diff))
	return 0, nil
}

// notifyReviewers wakes a project's reviewer agents that a PR is ready.
func (h *Hub) notifyReviewers(project, prID, worker string) {
	roster, err := h.store.For(project).Roster()
	if err != nil {
		return
	}
	for _, a := range roster {
		if a.Role == "reviewer" {
			name := a.Name
			go h.injectWhenReady(project, name, msgReviewReady(prID, worker))
		}
	}
}

// openPR resolves the PR a reviewer verb targets in a project: an explicit id, or
// the single oldest open PR when none is given.
func (h *Hub) openPR(project string, args []string) (store.PR, error) {
	ps := h.store.For(project)
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

// cmdApprove marks a PR approved (the human still merges — the only hard gate).
func (h *Hub) cmdApprove(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := h.store.For(c.Project)
	pr, err := h.openPR(c.Project, args)
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
	fmt.Fprintf(out, "%s approved — awaiting human merge ('sindri merge %s').\n", pr.ID, pr.ID)
	return 0, nil
}

// ApprovePR is the human approve path (TUI/CLI): marks a project's open PR approved.
func (h *Hub) ApprovePR(project, prID string) error {
	ps := h.store.For(project)
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
	h.notify()
	return nil
}

// RejectPR is the human reject path (TUI/CLI): sends the owning worker back to its
// branch to address the feedback and resubmit, with the [user] voice.
func (h *Hub) RejectPR(project, prID, feedback string) error {
	return h.reject(project, prID, feedback, true)
}

// fileList renders a blocking-files list for a user message.
func fileList(files []string) string {
	switch {
	case len(files) == 0:
		return "the conflicting files"
	case len(files) <= 5:
		return strings.Join(files, ", ")
	default:
		return strings.Join(files[:4], ", ") + fmt.Sprintf(", and %d more", len(files)-4)
	}
}

// reject rejects a project's PR with feedback and routes it to the owning worker
// (object-addressed). byUser selects the message's voice ([user] vs [reviewer]).
func (h *Hub) reject(project, prID, feedback string, byUser bool) error {
	ps := h.store.For(project)
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

	who, msg := "reviewer", msgRejectedByReviewer(pr.ID, feedback)
	if byUser {
		who, msg = "user", msgRejectedByUser(pr.ID, feedback)
	}
	_ = ps.LogPR(pr.ID, "rejected", "by "+who+": "+feedback)
	_ = ps.Log(pr.Agent, "reject", pr.ID+" ("+who+"): "+feedback)
	_ = h.injectWhenReady(project, pr.Agent, msg)
	h.notify()
	return nil
}

// MaterializeReview checks out a project's PR branch (detached) into its reserved
// .worktrees/review workspace, so a human can inspect it. Returns the path.
func (h *Hub) MaterializeReview(project, prID string) (string, error) {
	ps := h.store.For(project)
	root := h.projectRoot(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no such PR %q", prID)
	}
	path := filepath.Join(root, ".worktrees", "review")
	_ = git.WorktreeRemove(root, path) // fresh checkout each time
	if err := git.WorktreeAdd(root, path, pr.Branch); err != nil {
		return "", err
	}
	return path, nil
}

// LintPR runs the quality gate against a project's PR worktree and returns the
// output, headed with PASS/FAIL.
func (h *Hub) LintPR(project, prID string) (string, error) {
	ps := h.store.For(project)
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
	out, passed := h.runLint(filepath.Join(h.projectRoot(project), a.Workspace))
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

// cmdReject is the agent-reviewer reject command — rejects with the [reviewer] voice.
func (h *Hub) cmdReject(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		return 1, fmt.Errorf("usage: reject <pr-id> <feedback...>")
	}
	if err := h.reject(c.Project, args[0], strings.Join(args[1:], " "), false); err != nil {
		return 1, err
	}
	fmt.Fprintf(out, "%s rejected; worker notified.\n", args[0])
	return 0, nil
}

// cmdReview records a reviewer agent's verdict on a PR review it was assigned.
func (h *Hub) cmdReview(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) < 2 {
		return 1, fmt.Errorf("usage: review <pr-id> <pass|changes|fail> <findings...>")
	}
	ps := h.store.For(c.Project)
	prID, verdict := args[0], args[1]
	findings := strings.TrimSpace(strings.Join(args[2:], " "))
	revs, err := ps.Reviews(prID)
	if err != nil {
		return 1, err
	}
	for _, r := range revs {
		if r.Author == c.Agent && r.Verdict == "" { // your in-progress review
			if err := ps.RecordVerdict(r.ID, verdict, findings); err != nil {
				return 1, err
			}
			h.notify()
			fmt.Fprintf(out, "review recorded on %s: %s\n", prID, verdict)
			return 0, nil
		}
	}
	return 1, fmt.Errorf("no review assigned to you on %s", prID)
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
	// Bring the branch up to the current base first. A merely-stale branch rebases
	// silently; a CONFLICT is routed into the worker's resolution loop — the hub
	// leaves the conflict in the worker's workspace to edit (it can't run git) and
	// the branch returns for review once clean, rather than a dead-end "resubmit".
	if a, ok, _ := ps.GetAgent(pr.Agent); ok {
		wt := filepath.Join(root, a.Workspace)
		conflicts, done, err := git.RebaseStart(wt, pr.Branch, pr.Base)
		if err != nil {
			return store.PR{}, err
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
			return store.PR{}, fmt.Errorf("merge blocked: commit or stash your local changes to %s in the working checkout, then merge again (the PR is fine and stays approved)", files)
		}
		return store.PR{}, err
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

// rebasePlanners rebases every planner's branch in a project onto base after a
// merge. Best-effort: a dirty or conflicting worktree is logged and skipped.
func (h *Hub) rebasePlanners(project, base string) {
	ps := h.store.For(project)
	root := h.projectRoot(project)
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
		_ = h.injectWhenReady(project, a.Name, msgRebased(base))
	}
}

// MilestonePR opens (or refreshes) a milestone PR for the container an agent holds
// in a project, blocking the agent until the human merges.
func (h *Hub) MilestonePR(project, agent string) (store.PR, error) {
	ps := h.store.For(project)
	root := h.projectRoot(project)
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
	base, err := h.baseBranch(root)
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
	h.notify()
	return pr, nil
}

// resumeContainer puts a container's agent back to work after a milestone merge.
func (h *Hub) resumeContainer(project, agent string) {
	ps := h.store.For(project)
	st, _ := ps.GetState(agent)
	if st.Container == "" {
		return
	}
	if st.Task != "" {
		if t, ok, _ := ps.GetTask(st.Task); ok && (t.Status == "open" || t.Status == "in_progress") {
			_ = ps.SetState(store.AgentState{Agent: agent, Container: st.Container, Branch: st.Container, Task: st.Task, Phase: "working"})
			h.notify()
			return
		}
	}
	if _, ok := h.advanceContainer(project, agent, st.Container); !ok {
		_ = ps.SetState(store.AgentState{Agent: agent, Container: st.Container, Branch: st.Container, Phase: "idle"})
		h.notify()
	}
}
