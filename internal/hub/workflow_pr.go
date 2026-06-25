// package: hub / workflow (PR, review, merge)
// type:    logic (PR-as-merge-intent: submit → review → approve → host merge)
// job:     the reviewer verbs and the host merge; verdicts route to the owning
//
//	agent's session by branch (object-mediated, D-routing). git is hub-side.
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
	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// baseBranch reads the repo's base branch from the main checkout.
func (h *Hub) baseBranch() (string, error) { return git.CurrentBranch(h.root) }

// PRs returns all merge-intents (newest first).
func (h *Hub) PRs() ([]store.PR, error) { return h.store.PRs() }

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

// PRInfo returns a PR with its linked task and diff.
func (h *Hub) PRInfo(id string) (PRDetail, error) {
	pr, ok, err := h.store.GetPR(id)
	if err != nil {
		return PRDetail{}, err
	}
	if !ok {
		return PRDetail{}, fmt.Errorf("no such PR %q", id)
	}
	diff, _ := git.Diff(h.root, pr.Base, pr.Branch)
	task, _ := h.TaskInfo(pr.Task) // linked task; zero value if it can't be read
	reviews, _ := h.store.Reviews(id)
	lint, lintAt := h.store.GetPRLint(id)
	history, _ := h.store.PREvents(id)
	return PRDetail{PR: pr, Task: task, Diff: diff, Reviews: reviews, Lint: lint, LintAt: lintAt, History: history}, nil
}

// ReviewPrompt returns the default agentic-review instruction, read from
// .sindri/review-prompt.txt — auto-created with a built-in default if absent, so
// the user can edit the standard prompt in a plain text file.
func (h *Hub) ReviewPrompt() (string, error) {
	path := filepath.Join(h.root, ".sindri", "review-prompt.txt")
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

// RequestReview attaches a review requirement (free-text instruction) to a PR
// and dispatches it to a running reviewer agent — assigning the row and
// injecting the instruction. With no reviewer running, the requirement is
// recorded unassigned for one to pick up later.
func (h *Hub) RequestReview(prID, requirement string) error {
	pr, ok, err := h.store.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}
	requirement = strings.TrimSpace(requirement)
	if requirement == "" {
		requirement, _ = h.ReviewPrompt()
	}
	id, err := h.store.AddReview(prID, requirement)
	if err != nil {
		return err
	}
	if reviewer := h.runningReviewer(); reviewer != "" {
		if err := h.store.AssignReview(id, reviewer); err != nil {
			return err
		}
		_ = h.store.LogPR(prID, "review-requested", "assigned to "+reviewer)
		_ = h.assignedReviewInject(reviewer, pr, prID, requirement)
		h.notify()
		return nil
	}
	_ = h.store.LogPR(prID, "review-requested", "unassigned (no reviewer running)")
	h.notify()
	return nil
}

// assignedReviewInject checks the PR branch out into the reviewer's workspace and
// injects the precise, single-line review instruction.
func (h *Hub) assignedReviewInject(reviewer string, pr store.PR, prID, requirement string) error {
	// Check the PR's branch out (detached) into the reviewer's workspace, so it
	// can read the full code in context, build, and run — not just the diff.
	//
	// A failure here is LOUD, never silent: a reviewer that quietly falls back to
	// diff-only would be judging a stale base with no signal it ever happened. So
	// we record a host-visible PR history entry naming the failure, and the review
	// instruction below warns the reviewer that /workspace is NOT this PR.
	checkedOut := false
	a, ok, err := h.store.GetAgent(reviewer)
	switch {
	case err != nil:
		_ = h.store.LogPR(prID, "checkout-failed", fmt.Sprintf("reviewer %s lookup: %v", reviewer, err))
	case !ok:
		_ = h.store.LogPR(prID, "checkout-failed", "reviewer "+reviewer+" has no roster entry")
	default:
		if coErr := git.CheckoutDetached(filepath.Join(h.root, a.Workspace), pr.Branch); coErr != nil {
			_ = h.store.LogPR(prID, "checkout-failed", fmt.Sprintf("%s into %s: %v", pr.Branch, a.Workspace, coErr))
		} else {
			checkedOut = true
		}
	}
	return h.injectWhenReady(reviewer, msgReviewAssigned(prID, requirement, pr.Branch, pr.Base, checkedOut))
}

// runningReviewer returns the name of a live reviewer agent, or "".
func (h *Hub) runningReviewer() string {
	roster, _ := h.store.Roster()
	for _, a := range roster {
		if a.Role == "reviewer" && pod.Running(h.container(a.Name)) && h.sessionAlive(a.Name) {
			return a.Name
		}
	}
	return ""
}

// runLint runs the project's quality gates against a worktree by invoking
// `brokkr lint` there (a subprocess, so the concurrent hub never chdir's).
// The gate applies only to Go modules — a non-Go workspace has no Go gates and
// is skipped. openspec validation self-skips when openspec/ is absent.
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
	st, err := h.store.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Phase != "working" || st.Task == "" {
		fmt.Fprintln(out, replyNothingToSubmit)
		return 1, nil
	}
	a, _, _ := h.store.GetAgent(c.Agent)
	wt := filepath.Join(h.root, a.Workspace)
	// Lint gate (3.3): never accept a merge-intent for code that fails the
	// project's quality gates. Runs against the worktree before the PR exists, so
	// a failing worker just fixes and submits again.
	if lintOut, ok := h.runLint(wt); !ok {
		fmt.Fprintln(out, replyLintFail(strings.TrimSpace(lintOut)))
		_ = h.store.Log(c.Agent, "lint-fail", st.Task)
		return 1, nil
	}
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		msg = "work on " + st.Task
	}
	if err := git.CommitAll(wt, msg); err != nil {
		return 1, err
	}
	base, err := h.baseBranch()
	if err != nil {
		return 1, err
	}
	pr := store.PR{ID: "pr-" + st.Task, Task: st.Task, Agent: c.Agent, Branch: st.Branch, Base: base, Status: "open"}
	_, existed, _ := h.store.GetPR(pr.ID) // first submit vs a resubmit after rejection
	if err := h.store.PutPR(pr); err != nil {
		return 1, err
	}
	if err := h.store.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Phase: "submitted"}); err != nil {
		return 1, err
	}
	_ = h.store.Log(c.Agent, "submit", pr.ID)
	if existed {
		_ = h.store.LogPR(pr.ID, "resubmitted", "by "+c.Agent+": "+msg)
	} else {
		_ = h.store.LogPR(pr.ID, "created", "by "+c.Agent+": "+msg)
	}
	h.notifyReviewers(pr.ID, c.Agent)
	fmt.Fprintln(out, replyRegistered(pr.ID))
	return 0, nil
}

// cmdOpenspec is the planner's ship verb: `openspec submit [message]` turns its
// openspec edits into a PR — the same review→approve→merge cycle as a worker's,
// just on the planner's standing branch and with the mock todo id os-new (there's
// no backlog task behind it).
func (h *Hub) cmdOpenspec(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 || args[0] != "submit" {
		return 1, fmt.Errorf("usage: openspec submit [message]")
	}
	a, _, _ := h.store.GetAgent(c.Agent)
	wt := filepath.Join(h.root, a.Workspace)
	base, err := h.baseBranch()
	if err != nil {
		return 1, err
	}
	branch := plannerBranch(c.Agent)
	if !git.HasChanges(wt) && !git.Ahead(wt, base) {
		fmt.Fprintln(out, "Nothing to submit — edit /workspace/openspec first.")
		return 1, nil
	}
	if lintOut, ok := h.runLint(wt); !ok {
		fmt.Fprintln(out, replyLintFail(strings.TrimSpace(lintOut)))
		_ = h.store.Log(c.Agent, "lint-fail", branch)
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
	_, existed, _ := h.store.GetPR(pr.ID)
	if err := h.store.PutPR(pr); err != nil {
		return 1, err
	}
	if err := h.store.SetState(store.AgentState{Agent: c.Agent, Task: mockSpecTask, Branch: branch, Phase: "submitted"}); err != nil {
		return 1, err
	}
	_ = h.store.Log(c.Agent, "submit", pr.ID)
	if existed {
		_ = h.store.LogPR(pr.ID, "resubmitted", "by "+c.Agent+": "+msg)
	} else {
		_ = h.store.LogPR(pr.ID, "created", "by "+c.Agent+": "+msg)
	}
	h.notifyReviewers(pr.ID, c.Agent)
	fmt.Fprintln(out, replyRegistered(pr.ID))
	return 0, nil
}

// cmdShowPR prints a PR's metadata and diff so a reviewer can judge it.
func (h *Hub) cmdShowPR(_ registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		return 1, fmt.Errorf("usage: show <pr-id>")
	}
	pr, ok, err := h.store.GetPR(args[0])
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
	diff, err := git.Diff(h.root, pr.Base, pr.Branch)
	if err != nil {
		return 1, err
	}
	fmt.Fprintf(out, "\n%s\n", strings.TrimSpace(diff))
	return 0, nil
}

// notifyReviewers wakes reviewer agents that a PR is ready (object → agent
// routing: a PR ready for review routes to whoever can review it).
func (h *Hub) notifyReviewers(prID, worker string) {
	roster, err := h.store.Roster()
	if err != nil {
		return
	}
	for _, a := range roster {
		if a.Role == "reviewer" {
			name := a.Name
			go h.injectWhenReady(name, msgReviewReady(prID, worker))
		}
	}
}

// openPR resolves the PR a reviewer verb targets: an explicit id, or the single
// oldest open PR when none is given.
func (h *Hub) openPR(args []string) (store.PR, error) {
	if len(args) > 0 {
		pr, ok, err := h.store.GetPR(args[0])
		if err != nil {
			return store.PR{}, err
		}
		if !ok {
			return store.PR{}, fmt.Errorf("no such PR %q", args[0])
		}
		return pr, nil
	}
	open, err := h.store.PRs("open")
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
	pr, err := h.openPR(args)
	if err != nil {
		return 1, err
	}
	// Only an open PR can be approved. A PR the user rejected stays "rejected"
	// (the worker is told to stop, not resubmit), so a reviewer can never approve
	// over the user's rejection.
	if pr.Status != "open" {
		fmt.Fprintf(out, "%s is %s — only an open PR can be approved.\n", pr.ID, pr.Status)
		return 1, nil
	}
	pr.Status = "approved"
	if err := h.store.PutPR(pr); err != nil {
		return 1, err
	}
	_ = h.store.Log(c.Agent, "approve", pr.ID)
	_ = h.store.LogPR(pr.ID, "approved", "by "+c.Agent)
	fmt.Fprintf(out, "%s approved — awaiting human merge ('sindri merge %s').\n", pr.ID, pr.ID)
	return 0, nil
}

// ApprovePR is the human approve path (TUI/CLI) — the positive counterpart of
// RejectPR. It marks an open PR approved so the human can then merge it, with or
// without a reviewer agent in the loop (the merge gate is simply status ==
// approved). Only an open PR awaiting a verdict can be approved.
func (h *Hub) ApprovePR(prID string) error {
	pr, ok, err := h.store.GetPR(prID)
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
	if err := h.store.PutPR(pr); err != nil {
		return err
	}
	_ = h.store.LogPR(prID, "approved", "by user")
	h.notify()
	return nil
}

// RejectPR is the human reject path (TUI/CLI): sends the owning worker back to
// its branch to address the feedback and resubmit, with the [user] voice.
func (h *Hub) RejectPR(prID, feedback string) error { return h.reject(prID, feedback, true) }

// reject rejects a PR with feedback and routes it to the owning worker
// (object-addressed; the worker is never named by the rejecter). A rejection is
// "revise this", not "abandon it": whoever rejects — an agent reviewer
// (cmdReject) or the human (RejectPR) — the owner returns to its branch to
// address the feedback and resubmit. byUser only selects the message's voice
// ([user] vs [reviewer]) so the worker knows who asked.
func (h *Hub) reject(prID, feedback string, byUser bool) error {
	pr, ok, err := h.store.GetPR(prID)
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
	if err := h.store.PutPR(pr); err != nil {
		return err
	}
	// The owner returns to its branch to fix and resubmit. A worker goes back to
	// "working" its task; a planner has no backlog task, so it drops to its resting
	// phase (its directive stays the planner brief) — the injected message drives
	// the fix either way.
	phase := "working"
	if a, ok, _ := h.store.GetAgent(pr.Agent); ok && a.Role == "planner" {
		phase = restPhase(a.Role)
	}
	_ = h.store.SetState(store.AgentState{Agent: pr.Agent, Task: pr.Task, Branch: pr.Branch, Phase: phase})

	who, msg := "reviewer", msgRejectedByReviewer(pr.ID, feedback)
	if byUser {
		who, msg = "user", msgRejectedByUser(pr.ID, feedback)
	}
	_ = h.store.LogPR(pr.ID, "rejected", "by "+who+": "+feedback)
	_ = h.store.Log(pr.Agent, "reject", pr.ID+" ("+who+"): "+feedback)
	_ = h.injectWhenReady(pr.Agent, msg)
	h.notify()
	return nil
}

// MaterializeReview checks out a PR's branch (detached) into the reserved
// .worktrees/review workspace, so a human can inspect, build, and run it.
// Returns the path. Detached HEAD avoids conflicting with the agent's own
// worktree, which holds the same branch.
func (h *Hub) MaterializeReview(prID string) (string, error) {
	pr, ok, err := h.store.GetPR(prID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no such PR %q", prID)
	}
	path := filepath.Join(h.root, ".worktrees", "review")
	_ = git.WorktreeRemove(h.root, path) // fresh checkout each time
	if err := git.WorktreeAdd(h.root, path, pr.Branch); err != nil {
		return "", err
	}
	return path, nil
}

// LintPR runs the quality gate (`sindri lint all`) against a PR's worktree and
// returns the output, headed with PASS/FAIL.
func (h *Hub) LintPR(prID string) (string, error) {
	pr, ok, err := h.store.GetPR(prID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no such PR %q", prID)
	}
	a, ok, err := h.store.GetAgent(pr.Agent)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no agent %q for %s", pr.Agent, prID)
	}
	out, passed := h.runLint(filepath.Join(h.root, a.Workspace))
	status := "FAIL"
	if passed {
		status = "PASS"
	}
	if strings.TrimSpace(out) == "" {
		out = "(no output)\n"
	}
	result := fmt.Sprintf("lint %s\n\n%s", status, out)
	_ = h.store.SetPRLint(prID, result) // persist the latest result
	return result, nil
}

// cmdReject is the agent-reviewer reject command — rejects with the [reviewer] voice.
func (h *Hub) cmdReject(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		return 1, fmt.Errorf("usage: reject <pr-id> <feedback...>")
	}
	if err := h.reject(args[0], strings.Join(args[1:], " "), false); err != nil {
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
	prID, verdict := args[0], args[1]
	findings := strings.TrimSpace(strings.Join(args[2:], " "))
	revs, err := h.store.Reviews(prID)
	if err != nil {
		return 1, err
	}
	for _, r := range revs {
		if r.Author == c.Agent && r.Verdict == "" { // your in-progress review
			if err := h.store.RecordVerdict(r.ID, verdict, findings); err != nil {
				return 1, err
			}
			h.notify()
			fmt.Fprintf(out, "review recorded on %s: %s\n", prID, verdict)
			return 0, nil
		}
	}
	return 1, fmt.Errorf("no review assigned to you on %s", prID)
}

// Merge merges an approved PR into the base branch (host/human-only — the single
// hard gate), closes the task, frees the worker, and notifies it. Returns the
// PR for reporting.
func (h *Hub) Merge(prID string) (store.PR, error) {
	pr, ok, err := h.store.GetPR(prID)
	if err != nil {
		return store.PR{}, err
	}
	if !ok {
		return store.PR{}, fmt.Errorf("no such PR %q", prID)
	}
	if pr.Status != "approved" {
		return store.PR{}, fmt.Errorf("%s is %s — only an approved PR may be merged", prID, pr.Status)
	}
	// Bring the branch up to the current base first: a branch that merely fell
	// behind (base moved on under it) rebases cleanly and then merges without any
	// human step. A rebase CONFLICT is a genuine divergence — route it straight
	// back to the owning worker to resolve and resubmit, and stop the merge.
	if a, ok, _ := h.store.GetAgent(pr.Agent); ok {
		wt := filepath.Join(h.root, a.Workspace)
		if err := git.RebaseOnto(wt, pr.Branch, pr.Base); err != nil {
			fb := fmt.Sprintf("merge conflict: rebasing %s onto %s hit conflicts (%v). Resolve against the latest %s and resubmit.", pr.Branch, pr.Base, err, pr.Base)
			_ = h.reject(prID, fb, false) // object-addressed back to the worker
			return store.PR{}, fmt.Errorf("%s could not be merged — rebase onto %s conflicted; sent back to %s to resolve", prID, pr.Base, pr.Agent)
		}
	}
	if err := git.Merge(h.root, pr.Base, pr.Branch); err != nil {
		// The merge applies to the main checkout (where base lives). If that working
		// tree has uncommitted edits the merge would clobber, git refuses. That's not
		// the worker's doing — its branch is fine — so say so and point at the fix,
		// rather than dumping git's raw output or rejecting the PR.
		if e := err.Error(); strings.Contains(e, "would be overwritten") || strings.Contains(e, "commit your changes or stash") {
			return store.PR{}, fmt.Errorf("merge blocked: the main checkout (%s) has uncommitted local changes the merge would overwrite — commit or stash them there, then merge again (the PR branch itself is fine)", h.root)
		}
		return store.PR{}, err
	}
	pr.Status = "merged"
	if err := h.store.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	// Milestone PR for a held container: land the work but KEEP the branch and the
	// agent — fast-forward the container branch past the merge and resume the agent
	// on the container (it is not freed, the container task is not closed).
	if holder, _ := h.store.GetState(pr.Agent); holder.Container != "" && holder.Container == pr.Branch {
		if a, ok, _ := h.store.GetAgent(pr.Agent); ok {
			_ = git.RebaseOnto(filepath.Join(h.root, a.Workspace), pr.Branch, pr.Base) // ff past the merge
		}
		_ = h.store.Log(pr.Agent, "merged", prID+" (milestone)")
		_ = h.store.LogPR(prID, "merged", "milestone into "+pr.Base)
		h.resumeContainer(pr.Agent)
		_ = h.injectWhenReady(pr.Agent, msgMilestoneMerged(prID))
		h.rebasePlanners(pr.Base)
		h.notify()
		return pr, nil
	}
	if strings.HasPrefix(pr.Task, "td-") { // a planner's openspec PR has no real td task (os-new)
		if err := td.Close(h.root, pr.Task, "merged via "+prID); err != nil {
			fmt.Printf("warning: td close %s: %v\n", pr.Task, err)
		}
		_ = h.refreshTask(pr.Task)
	}
	rest := "idle"
	if a, ok, _ := h.store.GetAgent(pr.Agent); ok {
		rest = restPhase(a.Role)
	}
	_ = h.store.SetState(store.AgentState{Agent: pr.Agent, Phase: rest})
	_ = h.store.Log(pr.Agent, "merged", prID)
	_ = h.store.LogPR(prID, "merged", "into "+pr.Base)
	_ = h.injectWhenReady(pr.Agent, msgMerged(prID))
	h.rebasePlanners(pr.Base) // any merge moves base → keep planners current
	h.notify()
	return pr, nil
}

// rebasePlanners rebases every planner's branch onto base after a merge, so
// planners always see the latest code. Best-effort: a dirty or conflicting
// worktree is logged and skipped (the rebase is aborted, leaving it untouched).
func (h *Hub) rebasePlanners(base string) {
	roster, _ := h.store.Roster()
	for _, a := range roster {
		if a.Role != "planner" {
			continue
		}
		wt := filepath.Join(h.root, a.Workspace)
		if err := git.Rebase(wt, base); err != nil {
			_ = h.store.Log(a.Name, "rebase-skip", base+": "+err.Error())
			continue
		}
		_ = h.store.Log(a.Name, "rebase", "onto "+base)
		_ = h.injectWhenReady(a.Name, msgRebased(base))
	}
}

// MilestonePR opens (or refreshes) a milestone PR for the container an agent
// holds: it captures the current state of the container branch and blocks the
// agent until the human reviews and merges. The agent resumes the same container
// after the merge (its branch is not retired). Host-triggered, in coordination
// with the agent.
func (h *Hub) MilestonePR(agent string) (store.PR, error) {
	st, err := h.store.GetState(agent)
	if err != nil {
		return store.PR{}, err
	}
	if st.Container == "" {
		return store.PR{}, fmt.Errorf("%s isn't working a container — no milestone to open", agent)
	}
	a, ok, err := h.store.GetAgent(agent)
	if err != nil || !ok {
		return store.PR{}, fmt.Errorf("no such agent %q", agent)
	}
	wt := filepath.Join(h.root, a.Workspace)
	if err := git.CommitAll(wt, "milestone: "+st.Container); err != nil { // capture current state
		return store.PR{}, err
	}
	base, err := h.baseBranch()
	if err != nil {
		return store.PR{}, err
	}
	pr := store.PR{ID: "pr-" + st.Container, Task: st.Container, Agent: agent, Branch: st.Container, Base: base, Status: "open"}
	_, existed, _ := h.store.GetPR(pr.ID)
	if err := h.store.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	// Block the agent until the human merges (the one deliberate pause).
	if err := h.store.SetState(store.AgentState{Agent: agent, Container: st.Container, Branch: st.Container, Task: st.Task, Phase: "submitted"}); err != nil {
		return store.PR{}, err
	}
	if existed {
		_ = h.store.LogPR(pr.ID, "resubmitted", "milestone by "+agent)
	} else {
		_ = h.store.LogPR(pr.ID, "created", "milestone by "+agent)
	}
	_ = h.store.Log(agent, "milestone", pr.ID)
	h.notify()
	return pr, nil
}

// resumeContainer puts a container's agent back to work after a milestone merge:
// it continues the current subtask if it's still open, else advances to the next
// open child, else rests holding the container (awaiting more subtasks or the
// container's close).
func (h *Hub) resumeContainer(agent string) {
	st, _ := h.store.GetState(agent)
	if st.Container == "" {
		return
	}
	if st.Task != "" {
		if t, ok, _ := h.store.GetTask(st.Task); ok && (t.Status == "open" || t.Status == "in_progress") {
			_ = h.store.SetState(store.AgentState{Agent: agent, Container: st.Container, Branch: st.Container, Task: st.Task, Phase: "working"})
			h.notify()
			return
		}
	}
	if _, ok := h.advanceContainer(agent, st.Container); !ok {
		_ = h.store.SetState(store.AgentState{Agent: agent, Container: st.Container, Branch: st.Container, Phase: "idle"})
		h.notify()
	}
}
