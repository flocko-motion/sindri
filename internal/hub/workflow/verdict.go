// package: hub/workflow / verdict
// type:    logic (every way a PR's verdict changes: approve, reject, revoke)
// job:     the reviewer/planner/coauthor/human approve and reject paths, and a worker's
// own withdrawal — everything that writes pr.Status once a PR is up for review.
// limits:  verdicts only; submitting a PR is pr.go's, the host merge is merge.go's.
package workflow

import (
	"fmt"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// CmdApprove marks a PR approved (the human still merges — the only hard gate). A second, third…
// approval accumulates as another badge rather than being refused: approvals are evidence, not a
// single scalar the first verdict claims. A planner's is different in kind (-> plannerApprove).
func (e *Engine) CmdApprove(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := e.store.For(c.Project)
	pr, err := e.openPR(c.Project, args)
	if err != nil {
		return 1, err
	}
	if ownWork(pr, c) {
		fmt.Fprintln(out, ReplyNoSelfVerdict(pr.ID, "approve"))
		return 1, nil
	}
	if c.Role == "planner" {
		return e.plannerApprove(ps, c, pr, out)
	}
	if !api.PRApprovable(pr) {
		fmt.Fprintf(out, "%s is %s — only an open or already-approved PR can be approved.\n", pr.ID, pr.Status)
		return 1, nil
	}
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		return 1, err
	}
	_ = ps.Log(c.Agent, "approve", pr.ID)
	_ = ps.LogPR(pr.ID, "approved", "by "+c.Agent)
	if err := e.stampVerdict(c, pr.ID, "pass", ""); err != nil {
		return 1, fmt.Errorf("%s is approved, but recording who approved it failed: %w", pr.ID, err)
	}
	e.deps.Notify()
	fmt.Fprintf(out, "%s approved — awaiting human merge ('sindri merge %s').\n", pr.ID, pr.ID)
	return 0, nil
}

// ownWork reports whether the caller wrote the code under review. 05-workflow's self-review rule is
// about the COMMITS: a task it wrote is fine to rule on, its own branch never is.
func ownWork(pr store.PR, c registry.Caller) bool { return pr.Agent == c.Agent }

// stampVerdict records who ruled: on the review row a reviewer was assigned (-> completeReview), or
// outright for a coauthor, which holds no row and stays where it is — with the user, not in a queue.
func (e *Engine) stampVerdict(c registry.Caller, prID, verdict, findings string) error {
	if c.Role != "coauthor" {
		e.completeReview(c.Project, prID, c.Agent, verdict, findings)
		return nil
	}
	_, err := e.store.For(c.Project).AddVerdict(prID, "", c.Agent, verdict, findings, false)
	return err
}

// plannerApprove records a planner's badge: additional and optional, beside whatever a reviewer
// decides, never instead of it. It never touches pr.Status — self-review of a planner's own plan
// is not independent review, so its opinion can never by itself open the merge gate.
func (e *Engine) plannerApprove(ps *store.ProjectStore, c registry.Caller, pr store.PR, out io.Writer) (int, error) {
	if !api.PROpen(pr) {
		fmt.Fprintf(out, "%s is %s — its work is already settled, so a badge cannot be added.\n", pr.ID, pr.Status)
		return 1, nil
	}
	if _, err := ps.AddVerdict(pr.ID, "", c.Agent, "pass", "", true); err != nil {
		return 1, err
	}
	_ = ps.Log(c.Agent, "approve", pr.ID+" (advisory)")
	_ = ps.LogPR(pr.ID, "endorsed", "advisory approval by "+c.Agent)
	e.deps.Notify()
	fmt.Fprintf(out, "Recorded your advisory approval on %s — a second opinion beside the reviewer's, never a substitute for it.\n", pr.ID)
	return 0, nil
}

// completeReview stamps the verdict (a human verdict has no record), returns the reviewer to
// idle, and wakes it back into its loop.
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
	// Push only: asking `sindri` gets the reviewer exactly this — its next review — so nothing here
	// needs to survive being read late.
	_ = e.deps.Deliver(project, agent, MsgVerdictRecorded(prID), PushOnly)
}

// ApprovePR is the human approve path (TUI/CLI): marks a project's open (or already-approved) PR
// approved and records the badge — a human verdict otherwise left no trace in the reviews a
// PR's detail shows, only the bare status.
func (e *Engine) ApprovePR(project, prID string) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}
	if !api.PRApprovable(pr) {
		return fmt.Errorf("%s is %s — only an open or already-approved PR can be approved", prID, pr.Status)
	}
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		return err
	}
	if _, err := ps.AddVerdict(prID, "", "user", "pass", "", false); err != nil {
		return err
	}
	_ = ps.LogPR(prID, "approved", "by user")
	e.deps.Notify()
	return nil
}

// CmdRevoke withdraws the caller's own PR so it can keep working on the same branch. Without it the
// only route out of "submitted" was somebody else's verdict, so a worker that already knew its work
// was incomplete waited for a ruling on it — and nothing it wrote meanwhile was ever recorded.
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
	return e.reject(project, prID, feedback, api.SenderUser)
}

// reject routes feedback to the owning worker. voice is WHO ruled: "user", "reviewer", or a coauthor
// by name, which is also the author its badge carries.
func (e *Engine) reject(project, prID, feedback, voice string) error {
	ps := e.store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such PR %q", prID)
	}
	// Only a LANDED or discarded PR refuses a verdict (api.PROpen's line): an approved one still takes
	// a rejection, since overruling a reviewer to stop a merge is the point of a human verdict. A
	// verdict on merged work UNDID the merge in the record and looped the pair on an empty diff.
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
	// Only a reviewer has a row of its own to stamp (-> completeReview); everyone else's badge is this.
	if voice != "reviewer" {
		if _, err := ps.AddVerdict(prID, "", voice, "changes", feedback, false); err != nil {
			return err
		}
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

	who, msg := voice, MsgRejectedByAgent(voice, pr.ID)
	if voice == api.SenderUser {
		msg = MsgRejectedByUser(pr.ID)
	}
	if prior.Container != "" { // the milestone is the user's to re-open; there is nothing to re-submit
		msg = MsgMilestoneRejected(prior.Container, who)
	}
	_ = ps.LogPR(pr.ID, "rejected", "by "+who+": "+feedback)
	_ = ps.Log(pr.Agent, "reject", pr.ID+" ("+who+"): "+feedback)
	// From whoever ruled: an agent weights feedback by who it is from.
	_ = e.deps.Deliver(project, pr.Agent, msg, MailAndPush.From(who))
	e.deps.Notify()
	return nil
}

// CmdReject is an agent's reject: a "changes" verdict in the voice of whoever gave it — the role for
// a reviewer, its own name for a coauthor, which speaks for nobody but itself.
func (e *Engine) CmdReject(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: reject <pr-id> <feedback...>")
		return 2, nil
	}
	pr, ok, err := e.store.For(c.Project).GetPR(args[0])
	if err != nil {
		return 1, err
	}
	if ok && ownWork(pr, c) {
		fmt.Fprintln(out, ReplyNoSelfVerdict(pr.ID, "reject"))
		return 1, nil
	}
	feedback := strings.Join(args[1:], " ")
	if err := e.reject(c.Project, args[0], feedback, rejectVoice(c)); err != nil {
		return 1, err
	}
	if c.Role != "coauthor" { // its badge is reject's own, written under its name (-> reject)
		e.completeReview(c.Project, args[0], c.Agent, "changes", strings.TrimSpace(feedback))
	}
	fmt.Fprintf(out, "%s rejected; worker notified.\n", args[0])
	return 0, nil
}

// rejectVoice is the name a rejection speaks in — the reviewer's role, a coauthor's own name.
func rejectVoice(c registry.Caller) string {
	if c.Role == "coauthor" {
		return c.Agent
	}
	return "reviewer"
}
