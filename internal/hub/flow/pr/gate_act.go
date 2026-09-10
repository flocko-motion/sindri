// package: hub/flow/pr / gate_act
// type:    logic (the quality gate: one door, one queue, one verdict per commit)
// job:     record what an agent has as a COMMIT, ask the store whether that commit already has a
// verdict, and otherwise queue a run to produce one — then land the continuation the
// result unlocks (a PR, feedback to fix, a lint reply, a finding on an open PR).
// limits:  the gate check itself and what follows it. Queue mechanics (ranking, dequeue,
// one-at-a-time) live in hub/flow/run; the gate command in repo.
package pr

import (
	"context"
	"errors"
	"fmt"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/flow/run"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/world/task"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// GateCommit records what is loose in an agent's worktree and returns the commit to gate. First,
// because a verdict about a dirty tree names no sha and so can only ever be re-run — and because
// agents have no commit verb: this is where their work gets written down.
func (a *Act) GateCommit(project, agent, desc string) (sha string, err error) {
	ps := a.Store.For(project)
	ag, ok, err := ps.GetAgent(agent)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no such agent %q", agent)
	}
	wt := filepath.Join(a.Deps.ProjectRoot(project), ag.Workspace)
	if !git.HubOwnedTree(wt, ag.Workspace) {
		// A clean tree is still named by its HEAD — which is how a reviewer checking an untouched
		// checkout reuses the gate the submit already paid for. A dirty one is checked as it stands
		// and named by nothing, since a verdict filed under HEAD would describe a tree that isn't it.
		if dirty, derr := git.HasChanges(wt); derr != nil || dirty {
			return "", derr
		}
		return git.Head(wt)
	}
	st, _ := ps.GetState(agent)
	if err := git.CommitAll(wt, a.gateCommitMessage(ps, st, desc)); err != nil {
		return "", err
	}
	return git.Head(wt)
}

// gateCommitMessage composes what an inline gate's post-pass commit used to: the agent's summary,
// or the task's title.
func (a *Act) gateCommitMessage(ps *store.ProjectStore, st store.AgentState, desc string) string {
	target := st.Task
	if st.Container != "" {
		target = st.Container // a feature branch is what goes up, so it is what the commit is scoped to
	}
	tk, _, _ := ps.GetTask(target)
	desc = strings.TrimSpace(desc)
	if desc == "" {
		desc = tk.Title
	}
	if desc == "" {
		desc = "work on " + target
	}
	return task.ConventionalCommit(tk.Type, target, desc)
}

// GateRun opens a gate on sha: the run row, plus the shortcut that pays for the feature — a commit
// with a stored PASS settles here, unqueued, so lint-then-submit costs one gate.
func (a *Act) GateRun(project, agent, kind, message, sha string) (qr api.Run, reused bool, err error) {
	ps := a.Store.For(project)
	// Asked BEFORE the row exists, and the row then written already settled: a row that sits "queued"
	// for even an instant can be dequeued by the run watcher, and then one submit lands its
	// continuation twice — two PR log lines, two deliveries, and a phase decided by whichever finished last.
	out, reused := ps.GatePassed(sha, a.VerifyCmd(project))
	status := ""
	if reused {
		status = "passed"
	}
	qr, err = a.run().PutQueuedRun(project, store.Run{
		Agent: agent, Kind: kind, Command: "gate: " + kind + " @ " + run.GateSubject(sha), Message: message,
		Commit: sha, Status: status,
	})
	if err != nil {
		return api.Run{}, false, err
	}
	if !reused {
		return qr, false, nil
	}
	return qr, true, a.run().FinishRun(ps, project, qr, "passed", gateReusedReport(sha, true, out), 0, 0, 0)
}

// gateReport heads output with the commit it describes — without which no stored result is trustable.
func gateReport(sha string, passed bool, out string) string {
	verdict := "FAIL"
	if passed {
		verdict = "PASS"
	}
	if strings.TrimSpace(out) == "" {
		out = "(no output)\n"
	}
	return fmt.Sprintf("gate %s · %s\n\n%s", verdict, run.GateSubject(sha), out)
}

// gateReusedReport says plainly that nothing ran: a stored result reading like a fresh one costs
// somebody an hour later, chasing a check that never happened.
func gateReusedReport(sha string, passed bool, out string) string {
	return gateReport(sha, passed, out) +
		fmt.Sprintf("\ngate: reused — this is the verdict already recorded for %s, and nothing has "+
			"changed since, so nothing was re-run.\n", prompts.ShortSHA(sha))
}

// CmdLint is the self-check before submitting — the same gate at the same commit, so a pass here is
// the pass the submit reuses. With a pr-id it checks that PR instead: the reviewer's pre-verdict look.
func (a *Act) CmdLint(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) > 0 {
		res, err := a.lintPR(a.callerPRProject(c, args[0]), args[0], c.Agent)
		// An id this caller cannot reach is an ANSWER: reported as an error it reaches AgentExec as a
		// hub failure, which auto-escalates. balin was stopped for naming a PR outside the project its
		// review put it in — the sibling verbs learned this and lint was missed (-> core.ErrNoSuchPR).
		if errors.Is(err, core.ErrNoSuchPR) {
			fmt.Fprintln(out, prompts.ReplyNoSuchPR(args[0]))
			return 1, nil
		}
		if err != nil {
			return 1, err
		}
		fmt.Fprint(out, res)
		return 0, nil
	}
	sha, err := a.GateCommit(c.Project, c.Agent, "")
	if err != nil {
		return 1, err
	}
	// From the store, unqueued: the common case, and the whole reason for keying on the commit.
	if stored, ok := a.Store.For(c.Project).GatePassed(sha, a.VerifyCmd(c.Project)); ok {
		fmt.Fprint(out, gateReusedReport(sha, true, stored))
		return 0, nil
	}
	qr, _, err := a.GateRun(c.Project, c.Agent, run.GateLint, "", sha)
	if err != nil {
		return 1, err
	}
	fmt.Fprintln(out, prompts.ReplyLintQueued(qr.ID, run.GateSubject(sha), a.QueuePosition(qr.ID)))
	return 0, nil
}

// LintPR is the front-ends' door to the same check: the human asked, so nobody is messaged when it
// lands — they are watching the board it refreshes.
func (a *Act) LintPR(project, prID string) (string, error) {
	return a.lintPR(project, prID, api.SenderUser)
}

// lintPR checks a PR at the commit its branch names, reusing that commit's verdict when it has one —
// which right after a passing submit is the usual answer. Otherwise it queues, and says where it
// sits. asker is who is waiting: an agent is told when it lands, since it cannot watch the board.
func (a *Act) lintPR(project, prID, asker string) (string, error) {
	ps := a.Store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%w %q", core.ErrNoSuchPR, prID)
	}
	sha, err := git.BranchTip(a.Deps.ProjectRoot(project), pr.Branch)
	if err != nil {
		return "", err
	}
	// Pass OR fail: this is a READING, and the message that points a reviewer here must not cost the
	// fleet's only slot every time one looks at why a gate failed (-> prompts.MsgPRGateFinished). What may
	// never stand on a stored failure is a DECISION, and none is taken here.
	if stored, passed, ok := ps.GateVerdict(sha, a.VerifyCmd(project)); ok {
		report := gateReusedReport(sha, passed, stored)
		_ = ps.SetPRLint(prID, sha, report)
		return report, nil
	}
	// Asking twice must not queue twice: the second would hold the fleet's only slot for the same answer.
	qr, queued := a.openPRGate(ps, prID)
	if !queued {
		qr, err = a.run().PutRun(project, store.Run{
			Agent: asker, Kind: run.GateLintPR, Message: prID, Commit: sha,
			Command: "gate: lint " + prID + " @ " + prompts.ShortSHA(sha),
		})
		if err != nil {
			return "", err
		}
	}
	// Recorded per ASKER, not per run: whoever joins is waiting on a result too, and an agent that is
	// not on this list is parked on a message nobody will send (-> completeLintPR). The human is left
	// off it — they read the board, which this refreshes.
	if asker != api.SenderUser && asker != api.SenderSystem {
		if werr := ps.AddRunWaiter(qr.ID, asker); werr != nil {
			return "", werr
		}
	}
	// Stored, so the wait shows on the PR itself, not only in the pane of whoever asked.
	note := gateQueuedReport(qr, sha, a.QueuePosition(qr.ID))
	_ = ps.SetPRLint(prID, sha, note)
	a.Deps.Notify()
	return note, nil
}

// queuedPrecheck reports a check of this PR still WAITING. It reads current state when it runs, so a
// later sweep has nothing to add to it — while one already RUNNING answered the state it read, and a
// base that has moved since does need its own run.
func (a *Act) queuedPrecheck(ps *store.ProjectStore, prID string) bool {
	runs, err := ps.Runs("queued")
	if err != nil {
		return false
	}
	for _, r := range runs {
		if r.Kind == run.GatePrecheck && r.Message == prID {
			return true
		}
	}
	return false
}

// openPRGate finds a check of this PR already queued or running, so a second ask joins the first.
func (a *Act) openPRGate(ps *store.ProjectStore, prID string) (api.Run, bool) {
	runs, err := ps.Runs("queued", "running")
	if err != nil {
		return api.Run{}, false
	}
	for _, r := range runs {
		if r.Kind == run.GateLintPR && r.Message == prID {
			return r, true
		}
	}
	return api.Run{}, false
}

// gateQueuedReport is what a PR's pane shows while its gate waits — never a bare "linting…", which
// reads as already running.
func gateQueuedReport(r api.Run, sha string, position int) string {
	where := fmt.Sprintf("queued at position %d", position)
	if r.Status == "running" {
		where = "running now"
	}
	return fmt.Sprintf("gate %s · %s\n\nThe fleet runs one gate at a time (%s). "+
		"The result replaces this once it finishes.\n", r.Status, prompts.ShortSHA(sha), where)
}

// QueuePosition is a run's place in the fleet-wide queue, 0 once it is no longer waiting.
func (a *Act) QueuePosition(id string) int {
	all, err := a.Store.AllRuns("queued")
	if err != nil {
		return 0
	}
	return run.QueuePositions(all)[id]
}

// executeGateRun runs one gate from the queue, against the commit it names. git.GateTimeout bounds
// it (already equal to run.RunHardCap); nothing here adds a second bound.
func (a *Act) ExecuteGateRun(ctx context.Context, project string, r api.Run) error {
	ps := a.Store.For(project)
	if r.Kind == run.GatePrecheck {
		return a.executePrecheckRun(ctx, ps, project, r)
	}
	wt, cleanup, err := a.gateTree(project, r)
	defer cleanup()
	if err != nil {
		return a.run().FinishRun(ps, project, r, "failed", fmt.Sprintf("gate: could not prepare a tree to check: %s\n", err), 0, 0, -1)
	}
	if err := ps.SetRunStatus(r.ID, "running"); err != nil {
		return err
	}
	a.Deps.Notify()
	start := time.Now()
	out, passed := a.runGate(ctx, ps, project, wt, r.Commit)
	status, exitCode := "passed", 0
	if !passed {
		status, exitCode = "failed", 1
	}
	return a.run().FinishRun(ps, project, r, status, out, time.Since(start), run.RunHardCap, exitCode)
}

// runGate is the gate, recorded. That is the point: the next caller asking about this commit is
// answered from the store rather than building and testing it again.
func (a *Act) runGate(ctx context.Context, ps *store.ProjectStore, project, wt, sha string) (report string, passed bool) {
	verify := a.VerifyCmd(project)
	// The gate's own words are stored, not the report: the header naming the commit is composed for
	// each reader, so a reused result cannot end up carrying two of them.
	out, passed := git.Gate(ctx, wt, verify)
	_ = ps.SetGateResult(sha, passed, verify, out)
	return gateReport(sha, passed, out), passed
}

// gateOnce is the gate without the record — for a tree that exists only for this check (the
// preflight's combined replay), whose commit is thrown away with it, so nothing could ever reuse it.
func (a *Act) gateOnce(ctx context.Context, project, wt, sha string) (report string, passed bool) {
	out, passed := git.Gate(ctx, wt, a.VerifyCmd(project))
	return gateReport(sha, passed, out), passed
}

// gateTree checks the run's commit out fresh, never the live worktree it came from: a self-check
// parks nobody, so building that tree for minutes measures a moving target and files the result
// under a sha it was never taken on. The cost is a `worktree add` against a warm build cache.
func (a *Act) gateTree(project string, r api.Run) (wt string, cleanup func(), err error) {
	root := a.Deps.ProjectRoot(project)
	if r.Commit == "" {
		// No commit to check out: a tree the hub does not own (-> GateCommit), gated where it lies.
		ag, _, _ := a.Store.For(project).GetAgent(r.Agent)
		return filepath.Join(root, ag.Workspace), func() {}, nil
	}
	path, err := git.MaterializeGate(root, r.Commit)
	if err != nil {
		return "", func() {}, err
	}
	return path, func() { git.RemoveGate(root) }, nil
}

// completeGate is the continuation a gate result unlocks. Only "passed" creates a PR — the
// guarantee a failing PR is never created holds exactly the way it did when the gate ran inline.
func (a *Act) CompleteGate(project string, r api.Run, status, output string) error {
	ps := a.Store.For(project)
	switch {
	case r.Kind == run.GateLintPR:
		return a.completeLintPR(project, ps, r, output)
	case r.Kind == run.GatePrecheck:
		return nil // its record is the finding already logged on the PR, where a human reads it
	case status == "passed":
		return a.landGate(project, ps, r)
	case status == "failed":
		return a.rejectGate(project, ps, r, output)
	default: // timed_out, cancelled: the gate never reached a verdict — not a finding about the code
		return a.stallGate(project, ps, r, status)
	}
}

// completeLintPR stores the result where the PR view reads it, and tells EVERY agent that asked — one
// waiting on this cannot watch the board, and polling is the thing it must not do. The list, not the
// run's own asker: a second agent joins the queued run, and would otherwise never hear.
func (a *Act) completeLintPR(project string, ps *store.ProjectStore, r api.Run, output string) error {
	_ = ps.SetPRLint(r.Message, r.Commit, output)
	a.Deps.Notify()
	waiters, err := ps.RunWaiters(r.ID)
	if err != nil {
		return err
	}
	for _, agent := range waiters {
		_ = a.Harness.Say(project, agent, prompts.MsgPRGateFinished(r.Message, r.ID), mail.MailAndPush)
	}
	return nil
}

// landGate dispatches to the kind-specific continuation — the exact tail CmdSubmit/CmdContribute
// ran right after an inline gate passed, now re-derived from current state instead of a live call.
func (a *Act) landGate(project string, ps *store.ProjectStore, r api.Run) error {
	switch r.Kind {
	case run.GateContribute:
		_, err := a.openMilestoneOrInterim(project, r)
		return err
	case run.GateLint:
		return a.Harness.Say(project, r.Agent, prompts.MsgLintPassed(r.ID), mail.MailAndPush)
	default: // "submit"
		return a.landSubmit(project, ps, r)
	}
}

// openMilestoneOrInterim lands a passed contribute gate: the feature branch as a milestone if the
// agent holds one, otherwise this task's interim contribution — the same fork CmdContribute made
// before the gate became a queued step.
func (a *Act) openMilestoneOrInterim(project string, r api.Run) (store.PR, error) {
	ps := a.Store.For(project)
	st, err := ps.GetState(r.Agent)
	if err != nil {
		return store.PR{}, err
	}
	if st.Container != "" {
		pr, err := a.openMilestone(project, r.Agent, r.Message)
		if err != nil {
			return store.PR{}, err
		}
		// Push only: it names what just went up, which nothing else states, but it is nothing to
		// act on now — just wait — so it need not survive being read late.
		_ = a.Harness.Say(project, r.Agent, "[hub] "+prompts.ReplyMilestoneContributed(pr.ID, st.Container), mail.PushOnly)
		return pr, nil
	}
	root := a.Deps.ProjectRoot(project)
	ag, _, _ := ps.GetAgent(r.Agent)
	wt := filepath.Join(root, ag.Workspace)
	base, err := a.BaseBranch(root)
	if err != nil {
		return store.PR{}, err
	}
	pr := store.PR{ID: "pr-" + st.Task, Task: st.Task, Agent: r.Agent, Branch: st.Branch, Base: base, Status: "open", Kind: "interim"}
	_, existed, _ := ps.GetPR(pr.ID)
	if err := ps.PutPR(pr); err != nil {
		return store.PR{}, err
	}
	// A conflict here would need the resolve loop, exactly as it does inline — but the gate already
	// ran against the tree this rebase now replays, so the rare conflict left here still lands the
	// agent in "resolving" the same way, just one step later than an inline gate would have.
	conflicts, done, err := git.RebaseStep(wt, st.Branch, base)
	if err != nil {
		return store.PR{}, err
	}
	if !done {
		_ = ps.SetState(store.AgentState{Agent: r.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "resolving"},
			store.ReasonAdvanced, "contribute rebase conflicts: "+strings.Join(conflicts, ", "))
		_ = ps.Log(r.Agent, "contribute-conflict", strings.Join(conflicts, ", "))
		_ = a.Harness.Say(project, r.Agent, "[hub] "+prompts.ReplyContributeConflicts(base, conflicts), mail.MailAndPush)
		return pr, nil
	}
	if err := ps.SetState(store.AgentState{Agent: r.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "submitted"},
		store.ReasonAdvanced, "interim contribution submitted: "+pr.ID); err != nil {
		return store.PR{}, err
	}
	_ = ps.Log(r.Agent, "contribute", pr.ID)
	msg := a.gateCommitMessage(ps, st, r.Message)
	if existed {
		_ = ps.LogPR(pr.ID, "resubmitted", "interim, by "+r.Agent+": "+msg)
	} else {
		_ = ps.LogPR(pr.ID, "created", "interim, by "+r.Agent+": "+msg)
	}
	a.Deps.Notify()
	// Push only, same reason as the milestone case above.
	_ = a.Harness.Say(project, r.Agent, "[hub] "+prompts.ReplyContributed(pr.ID), mail.PushOnly)
	return pr, nil
}

// landSubmit lands a passed submit gate: record the PR, request review, tell the agent. The commit is
// what was gated (-> GateCommit), so nothing is committed here.
func (a *Act) landSubmit(project string, ps *store.ProjectStore, r api.Run) error {
	st, err := ps.GetState(r.Agent)
	if err != nil {
		return err
	}
	target, branch := st.Task, st.Branch
	if st.Container != "" {
		target, branch = st.Container, st.Container
	}
	// The task can close while the gate runs — a checkpoint closed one 126 seconds before its own
	// submit landed, and the PR that resulted drew two full reviews it could never act on. This is
	// the only point that shuts that window, since the gate is what takes the time.
	if t, ok, terr := ps.GetTask(target); terr == nil && ok && !api.Open(t) {
		_ = ps.Log(r.Agent, "submit-refused", target+" closed while the gate ran")
		return a.Harness.Say(project, r.Agent, prompts.MsgSubmitTaskClosed(target), mail.MailAndPush)
	}
	base, err := a.BaseBranch(a.Deps.ProjectRoot(project))
	if err != nil {
		return err
	}
	pr := store.PR{ID: "pr-" + target, Task: target, Agent: r.Agent, Branch: branch, Base: base, Status: "open"}
	_, existed, _ := ps.GetPR(pr.ID)
	if err := ps.PutPR(pr); err != nil {
		return err
	}
	if err := ps.SetState(store.AgentState{
		Agent: r.Agent, Task: st.Task, Branch: branch, Container: st.Container, Phase: "submitted",
	}, store.ReasonAdvanced, "submitted: "+pr.ID); err != nil {
		return err
	}
	_ = ps.Log(r.Agent, "submit", pr.ID)
	msg := a.gateCommitMessage(ps, st, r.Message)
	// On the TASK THREAD in the author's own name, where the reviewer's brief already sends it — so
	// the answers are read, and a defect where the author said it swept is the sharpest finding there
	// is. By ATTEMPT: answering sends the author into the code, so the tree moves underneath. Closed
	// here, so a resubmission after a rejection is asked its own questions.
	if open, answers, aerr := ps.OpenSubmitAnswers(r.Agent); aerr == nil && open != "" {
		for _, comment := range prompts.SubmitAnswerComments(answers, r.Agent) {
			_ = a.Deps.AddTaskComment(project, target, r.Agent, comment)
		}
		_ = ps.FinishSubmitAnswers(r.Agent, open)
	}
	if existed {
		_ = ps.LogPR(pr.ID, "resubmitted", "by "+r.Agent+": "+msg)
	} else {
		_ = ps.LogPR(pr.ID, "created", "by "+r.Agent+": "+msg)
	}
	if err := a.RequestReview(project, pr.ID, ""); err != nil {
		_ = ps.Log(r.Agent, "review-request-failed", pr.ID+": "+err.Error())
		return a.Harness.Say(project, r.Agent, "[hub] "+prompts.ReplyReviewRequestFailed(pr.ID, err), mail.MailAndPush)
	}
	// No "now up for review" message: it changes nothing the agent does — it waits either way —
	// and the verdict that eventually arrives (a merge push, or a mailed rejection) says it all.
	return nil
}

// rejectGate lands a failed gate: back to "working" with the violations, exactly what an inline
// refusal left the agent to fix — only the delivery (injected, not a command reply) differs.
func (a *Act) rejectGate(project string, ps *store.ProjectStore, r api.Run, output string) error {
	// Not a finding about the diff: only the user can set `verify:`, so "fix the violations" would
	// send the agent hunting its own work for a fault that is not there.
	if strings.TrimSpace(output) == strings.TrimSpace(git.MsgNoGate) {
		return a.escalateNoGate(project, ps, r)
	}
	st, err := a.backToWorking(ps, r)
	if err != nil {
		return err
	}
	_ = ps.Log(r.Agent, "lint-fail", gateTarget(st))
	a.Deps.Notify()
	// MAIL: a gate report is what must be READ, re-readable beside the code and spent when the agent
	// chooses. Pushed, two pages of console output were typed into a live session and charged to its
	// context at once. Announced within a sweep anyway (-> NudgeMailWaiting), so nothing waits.
	return a.Harness.Say(project, r.Agent, prompts.MsgGateFailed(strings.TrimSpace(output)), mail.MailOnly)
}

// escalateNoGate stops the agent on the one question it cannot answer. Escalated rather than told:
// nothing it does next can land, and the escalation is what reaches the user.
func (a *Act) escalateNoGate(project string, ps *store.ProjectStore, r api.Run) error {
	if _, err := a.Deps.Escalate(project, r.Agent, prompts.MsgNoGateQuestion); err != nil {
		return err
	}
	_ = ps.Log(r.Agent, "gate-unconfigured", gateTarget(mustState(ps, r.Agent)))
	a.Deps.Notify()
	// Ungated by construction: this push explains the very escalation that would otherwise hold it.
	return a.Harness.Say(project, r.Agent, prompts.MsgNoGateEscalated(git.MsgNoGate), mail.MailAndPush)
}

// mustState reads a state row where its absence is not actionable: the caller is only reporting.
func mustState(ps *store.ProjectStore, agent string) store.AgentState {
	st, _ := ps.GetState(agent)
	return st
}

// stallGate lands a gate that never reached a verdict: back to "working", told to just try again
// — never as a lint failure, or a flaky timeout reads as a violation and an agent "fixes" nothing.
func (a *Act) stallGate(project string, ps *store.ProjectStore, r api.Run, status string) error {
	if _, err := a.backToWorking(ps, r); err != nil {
		return err
	}
	_ = ps.Log(r.Agent, "gate-incomplete", status)
	a.Deps.Notify()
	if r.Kind == run.GateLint {
		// "Submit again" is the wrong instruction for a check nobody submitted: the agent is still
		// working, and what it lost is the answer, not the attempt.
		return a.Harness.Say(project, r.Agent, prompts.MsgLintIncomplete(status), mail.MailAndPush)
	}
	return a.Harness.Say(project, r.Agent, prompts.MsgGateIncomplete(status), mail.MailAndPush)
}

// backToWorking releases an agent a landing gate parked. A self-check is exempt: it parked nobody, so
// a phase written here would overwrite whatever the agent moved on to while it waited.
func (a *Act) backToWorking(ps *store.ProjectStore, r api.Run) (store.AgentState, error) {
	st, _ := ps.GetState(r.Agent)
	if r.Kind == run.GateLint {
		return st, nil
	}
	return st, ps.SetState(store.AgentState{
		Agent: r.Agent, Task: st.Task, Branch: st.Branch, Container: st.Container, Phase: "working",
	}, store.ReasonRejected, "gate failed: "+gateTarget(st))
}

// gateTarget names what a gate was checking, for the activity log — the container if the agent
// holds one, the task otherwise.
func gateTarget(st store.AgentState) string {
	if st.Container != "" {
		return st.Container
	}
	return st.Task
}
