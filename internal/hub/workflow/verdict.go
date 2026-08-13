// package: hub/workflow / verdict
// type:    logic (every way a PR's verdict changes: approve, reject, revoke)
// job:     the reviewer/planner/human approve and reject paths, and a worker's own
// withdrawal — everything that writes pr.Status once a PR is up for review.
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
	e.completeReview(c.Project, pr.ID, c.Agent, "pass", "") // record the verdict + return the reviewer to idle
	e.deps.Notify()
	fmt.Fprintf(out, "%s approved — awaiting human merge ('sindri merge %s').\n", pr.ID, pr.ID)
	return 0, nil
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
	_ = e.deps.InjectWhenReady(project, agent, MsgVerdictRecorded(prID))
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
	if byUser { // the agent path already gets its badge from completeReview, right after this call
		if _, err := ps.AddVerdict(prID, "", "user", "changes", feedback, false); err != nil {
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
