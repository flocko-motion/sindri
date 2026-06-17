// package: hub / workflow
// type:    logic (the act → report → idle loop + PR-as-merge-intent)
// job:     the real worker/reviewer verbs and the host merge. Tasks are a cached
//
//	read model synced from td (D15); `next` claims one and branches;
//	`submit` records a merge-intent and returns (no blocking); the
//	reviewer approves/rejects; the human merges. Verdicts are routed to
//	the owning agent's session by branch (object-mediated, D-routing).
//
// limits:  git is entirely hub-side (the agent edits /workspace, the hub commits
//
//	and merges); writes to td go through the td adapter (D15).
package hub

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/issue"
)

// baseBranch reads the repo's base branch from the main checkout.
func (h *Hub) baseBranch() (string, error) { return git.CurrentBranch(h.root) }

// PRs returns all merge-intents (newest first).
func (h *Hub) PRs() ([]store.PR, error) { return h.store.PRs() }

// PRDetail is a merge-intent plus its linked task and diff (for `pr info`).
type PRDetail struct {
	PR   store.PR   `json:"pr"`
	Task store.Task `json:"task"`
	Diff string     `json:"diff"`
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
	return PRDetail{PR: pr, Task: task, Diff: diff}, nil
}

// Tasks refreshes from td and returns all cached tasks (for `task list`).
func (h *Hub) Tasks() ([]store.Task, error) {
	_ = h.SyncTasks() // best-effort; fall back to cache on failure
	return h.store.AllTasks()
}

// TaskInfo returns one task, refreshed from the source of truth first (D15).
func (h *Hub) TaskInfo(id string) (store.Task, error) {
	t, err := td.Get(h.root, id)
	if err != nil {
		return store.Task{}, err
	}
	st := toStoreTask(t)
	if d, a, derr := td.Detail(h.root, id); derr == nil {
		st.Description, st.Acceptance = d, a
	}
	_ = h.store.UpsertTask(st)
	return st, nil
}

// NewTask creates a task via the td tool and returns its id.
func (h *Hub) NewTask(title, typ, priority string, labels []string) (string, error) {
	out, err := td.Create(h.root, title, td.CreateOpts{Type: typ, Priority: priority, Labels: labels})
	if err != nil {
		return "", err
	}
	_ = h.SyncTasks()
	h.notify()
	// td prints e.g. "CREATED td-1add0f" — return just the id.
	id := strings.TrimSpace(out)
	for _, f := range strings.Fields(out) {
		if strings.HasPrefix(f, "td-") {
			id = f
			break
		}
	}
	return id, nil
}

// runLint runs the project's quality gates against a worktree by invoking
// `sindri lint all` there (a subprocess, so the concurrent hub never chdir's).
// The gate applies only to Go modules — a non-Go workspace has no Go gates and
// is skipped. openspec validation self-skips when openspec/ is absent.
func (h *Hub) runLint(wt string) (output string, ok bool) {
	if _, err := os.Stat(filepath.Join(wt, "go.mod")); err != nil {
		return "", true // no Go module — no lint gate applies
	}
	self, err := os.Executable()
	if err != nil {
		return "lint: " + err.Error(), false
	}
	cmd := exec.Command(self, "lint", "all")
	cmd.Dir = wt
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// SyncTasks refreshes the whole cached task set from td (the sync read path).
// Caches all tasks (every status) so UIs can filter open/closed/all client-side.
func (h *Hub) SyncTasks() error {
	tasks, err := td.Tasks(h.root, issue.FilterAll)
	if err != nil {
		return err
	}
	rows := make([]store.Task, len(tasks))
	for i, t := range tasks {
		rows[i] = toStoreTask(t)
	}
	return h.store.ReplaceTasks(rows)
}

func (h *Hub) refreshTask(id string) error {
	t, err := td.Get(h.root, id)
	if err != nil {
		return err
	}
	return h.store.UpsertTask(toStoreTask(t))
}

func toStoreTask(t issue.Task) store.Task {
	return store.Task{
		ID: t.ID, Title: t.Title, Status: t.Status, Priority: t.Priority,
		Type: t.Type, Labels: strings.Join(t.Labels, ","), ParentID: t.ParentID,
	}
}

// cmdNext claims the highest-priority open task for a worker and branches for it.
// Tasks are refreshed from the source of truth first (refresh-before-assignment,
// D15) so a stale/closed task is never handed out.
func (h *Hub) cmdNext(c registry.Caller, _ []string, out io.Writer) (int, error) {
	if err := h.SyncTasks(); err != nil {
		fmt.Fprintf(out, "warning: task sync failed (%v) — using cached tasks\n", err)
	}
	open, err := h.store.OpenTasks()
	if err != nil {
		return 1, err
	}
	if len(open) == 0 {
		fmt.Fprintln(out, "No open tasks. Wait — the hub will tell you when there is work.")
		return 0, nil
	}
	t := open[0]
	base, err := h.baseBranch()
	if err != nil {
		return 1, err
	}
	a, ok, err := h.store.GetAgent(c.Agent)
	if err != nil || !ok {
		return 1, fmt.Errorf("agent %s missing: %v", c.Agent, err)
	}
	wt := filepath.Join(h.root, a.Workspace)
	branch := t.ID
	if err := td.SetStatus(h.root, t.ID, "in_progress"); err != nil {
		return 1, err
	}
	_ = h.refreshTask(t.ID)
	if err := git.CreateBranch(wt, branch, base); err != nil {
		return 1, err
	}
	if err := h.store.SetState(store.AgentState{Agent: c.Agent, Task: t.ID, Branch: branch, Phase: "working"}); err != nil {
		return 1, err
	}
	_ = h.store.Log(c.Agent, "claim", t.ID+" "+t.Title)
	fmt.Fprintf(out, "Claimed %s: %s\nBranch:  %s (your /workspace)\nWhen done, run 'sindri-worker submit'.\n", t.ID, t.Title, branch)
	return 0, nil
}

// cmdSubmit commits the worker's worktree, records a merge-intent, and returns
// immediately — the worker then goes idle until the hub injects a verdict (D5).
func (h *Hub) cmdSubmit(c registry.Caller, args []string, out io.Writer) (int, error) {
	st, err := h.store.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	if st.Phase != "working" || st.Task == "" {
		fmt.Fprintln(out, "Nothing to submit — run 'sindri-worker next' to pick up a task first.")
		return 1, nil
	}
	a, _, _ := h.store.GetAgent(c.Agent)
	wt := filepath.Join(h.root, a.Workspace)
	// Lint gate (3.3): never accept a merge-intent for code that fails the
	// project's quality gates. Runs against the worktree before the PR exists, so
	// a failing worker just fixes and submits again.
	if lintOut, ok := h.runLint(wt); !ok {
		fmt.Fprintf(out, "Lint failed — fix the violations and submit again:\n%s\n", strings.TrimSpace(lintOut))
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
	if err := h.store.PutPR(pr); err != nil {
		return 1, err
	}
	if err := h.store.SetState(store.AgentState{Agent: c.Agent, Task: st.Task, Branch: st.Branch, Phase: "submitted"}); err != nil {
		return 1, err
	}
	_ = h.store.Log(c.Agent, "submit", pr.ID)
	h.notifyReviewers(pr.ID, c.Agent)
	fmt.Fprintf(out, "%s registered. You'll be informed when it's reviewed. Please wait — this may take a while.\n", pr.ID)
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
			go h.injectWhenReady(name, fmt.Sprintf(
				"[hub] %s from %s is ready for review. Run `sindri-worker show %s`, then `approve %s` or `reject %s <feedback>`.",
				prID, worker, prID, prID, prID))
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
	pr.Status = "approved"
	if err := h.store.PutPR(pr); err != nil {
		return 1, err
	}
	_ = h.store.Log(c.Agent, "approve", pr.ID)
	fmt.Fprintf(out, "%s approved — awaiting human merge ('sindri merge %s').\n", pr.ID, pr.ID)
	return 0, nil
}

// cmdReject rejects a PR with feedback and routes that feedback to the owning
// worker's session (object-addressed; the reviewer never names the worker).
func (h *Hub) cmdReject(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		return 1, fmt.Errorf("usage: reject <pr-id> <feedback...>")
	}
	pr, ok, err := h.store.GetPR(args[0])
	if err != nil {
		return 1, err
	}
	if !ok {
		return 1, fmt.Errorf("no such PR %q", args[0])
	}
	feedback := strings.TrimSpace(strings.Join(args[1:], " "))
	if feedback == "" {
		feedback = "changes requested"
	}
	pr.Status, pr.Feedback = "rejected", feedback
	if err := h.store.PutPR(pr); err != nil {
		return 1, err
	}
	// The owning worker returns to working on the same branch.
	_ = h.store.SetState(store.AgentState{Agent: pr.Agent, Task: pr.Task, Branch: pr.Branch, Phase: "working"})
	_ = h.store.Log(c.Agent, "reject", pr.ID+": "+feedback)
	// Object-mediated routing: resolve branch → owning agent → inject.
	_ = h.injectWhenReady(pr.Agent, fmt.Sprintf("[reviewer] %s rejected: %s — please fix and 'sindri-worker submit' again.", pr.ID, feedback))
	fmt.Fprintf(out, "%s rejected; %s notified.\n", pr.ID, pr.Agent)
	return 0, nil
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
	if err := git.Merge(h.root, pr.Base, pr.Branch); err != nil {
		return store.PR{}, err
	}
	pr.Status = "merged"
	if err := h.store.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	if err := td.Close(h.root, pr.Task, "merged via "+prID); err != nil {
		fmt.Printf("warning: td close %s: %v\n", pr.Task, err)
	}
	_ = h.refreshTask(pr.Task)
	_ = h.store.SetState(store.AgentState{Agent: pr.Agent, Phase: "idle"})
	_ = h.store.Log(pr.Agent, "merged", prID)
	_ = h.injectWhenReady(pr.Agent, fmt.Sprintf("[hub] %s merged. Run 'sindri-worker next' for the next task.", prID))
	h.notify()
	return pr, nil
}
