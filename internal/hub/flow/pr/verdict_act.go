// package: hub/flow/pr / verdict_act
// type:    logic (every way a PR's verdict changes: approve, reject, revoke)
// job:     the reviewer/planner/coauthor/human approve and reject paths, and a worker's
// own withdrawal — everything that writes pr.Status once a PR is up for review.
// limits:  verdicts only; submitting a PR is pr_act.go's, the host merge is merge_act.go's.
package pr

import (
	"errors"
	"fmt"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// CmdApprove marks a PR approved (the human still merges — the only hard gate). A second, third…
// approval accumulates as another badge rather than being refused: approvals are evidence, not a
// single scalar the first verdict claims. A planner's is different in kind (-> plannerApprove).
func (a *Act) CmdApprove(c registry.Caller, args []string, out io.Writer) (int, error) {
	pr, err := a.openPR(c, args)
	switch {
	case errors.Is(err, core.ErrNoSuchPR):
		fmt.Fprintln(out, prompts.ReplyNoSuchPR(args[0]))
		return 1, nil
	case errors.Is(err, core.ErrNoOpenPRs):
		fmt.Fprintln(out, "Nothing is up for review here, so there is nothing to approve. Run `sindri` for your next move.")
		return 1, nil
	case err != nil:
		return 1, err
	}
	if ownWork(pr, c) {
		fmt.Fprintln(out, prompts.ReplyNoSelfVerdict(pr.ID, "approve"))
		return 1, nil
	}
	// pr.Project, not c.Project: a api.GlobalProject reviewer's own project never holds the PR it
	// approves — the PR record stays with its own project regardless of who is ruling on it.
	ps := a.Store.For(pr.Project)
	if c.Role == "planner" {
		return a.plannerApprove(ps, c, pr, out)
	}
	// approveVoice, not the caller's name: a reviewer's badge is its review row, stamped below.
	if _, err := a.approve(pr.Project, pr.ID, approveVoice(c)); err != nil {
		fmt.Fprintf(out, "%v\n", err) // an already-settled PR is the agent's to read, not a hub fault
		return 1, nil
	}
	_ = a.Store.For(c.Project).Log(c.Agent, "approve", pr.ID)
	if c.Role != "coauthor" { // its badge is approve's own, written under its name (-> approve)
		a.completeReview(pr.Project, c.Project, pr.ID, c.Agent, "pass", "")
	}
	fmt.Fprintf(out, "%s approved — awaiting human merge ('sindri merge %s').\n", pr.ID, pr.ID)
	return 0, nil
}

// approveVoice is the name an approval speaks in — the reviewer's role, a coauthor's own name. The
// same division rejectVoice draws, so one PR's badges read alike whichever verdict landed.
func approveVoice(c registry.Caller) string {
	if c.Role == "coauthor" {
		return c.Agent
	}
	return "reviewer"
}

// ownWork reports whether the caller wrote the code under review. 05-workflow's self-review rule is
// about the COMMITS: a task it wrote is fine to rule on, its own branch never is.
func ownWork(pr store.PR, c registry.Caller) bool { return pr.Agent == c.Agent }

// plannerApprove records a planner's badge: additional and optional, beside whatever a reviewer
// decides, never instead of it. It never touches pr.Status — self-review of a planner's own plan
// is not independent review, so its opinion can never by itself open the merge gate.
func (a *Act) plannerApprove(ps *store.ProjectStore, c registry.Caller, pr store.PR, out io.Writer) (int, error) {
	if !api.PROpen(pr) {
		fmt.Fprintf(out, "%s is %s — its work is already settled, so a badge cannot be added.\n", pr.ID, pr.Status)
		return 1, nil
	}
	if _, err := ps.AddVerdict(pr.ID, "", c.Agent, "pass", "", true); err != nil {
		return 1, err
	}
	_ = ps.Log(c.Agent, "approve", pr.ID+" (advisory)")
	_ = ps.LogPR(pr.ID, "endorsed", "advisory approval by "+c.Agent)
	a.Deps.Notify()
	fmt.Fprintf(out, "Recorded your advisory approval on %s — a second opinion beside the reviewer's, never a substitute for it.\n", pr.ID)
	return 0, nil
}

// completeReview stamps the verdict on the review row filed under prProject — never home, for a
// api.GlobalProject reviewer — and frees the reviewer. The session is the reviewer's map's to clear.
func (a *Act) completeReview(prProject, home, prID, agent, verdict, findings string) {
	if revs, err := a.Store.For(prProject).Reviews(prID); err == nil {
		for _, r := range revs {
			if r.Author == agent && r.Verdict == "" {
				_ = a.Store.For(prProject).RecordVerdict(r.ID, verdict, findings)
				break
			}
		}
	}
	// The verdict recorded IS the release: its own map reads a review it no longer holds
	// (-> cond.ReviewDone), and the next pull request arrives as its own hand-over.
	a.Flow.Wake(home, agent, topic.PRVerdict)
}

// ApprovePR is the human approve path (TUI/CLI), and the agent's own is CmdApprove: both are the
// one operation below, told who is asking.
func (a *Act) ApprovePR(project, prID string) error {
	_, err := a.approve(project, prID, api.SenderUser)
	return err
}

// approve opens the merge gate on one PR in the voice of whoever ruled — "user", "reviewer", or a
// coauthor by name, which is also the author its badge carries. The one implementation, as reject's
// has been: written twice, the human path silently lacked the guards and the log entry the agent's
// had, and nothing kept the pair in step.
//
// Only a reviewer has a review row of its own to stamp (-> completeReview); everyone else's badge is
// the verdict written here. The caller-specific parts — refusing self-review, a planner's advisory
// badge, freeing the reviewer — stay with the caller that has them.
func (a *Act) approve(project, prID, voice string) (store.PR, error) {
	ps := a.Store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return store.PR{}, err
	}
	if !ok {
		return store.PR{}, fmt.Errorf("%w %q", core.ErrNoSuchPR, prID)
	}
	if !api.PRApprovable(pr) {
		return pr, fmt.Errorf("%s is %s — only an open or already-approved PR can be approved", prID, pr.Status)
	}
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		return pr, err
	}
	if voice != "reviewer" {
		if _, err := ps.AddVerdict(prID, "", voice, "pass", "", false); err != nil {
			return pr, err
		}
	}
	_ = ps.LogPR(prID, "approved", "by "+voice)
	a.Deps.Notify()
	return pr, nil
}

// CmdRevoke withdraws the caller's own PR so it can keep working on the same branch. Without it the
// only route out of "submitted" was somebody else's verdict, so a worker that already knew its work
// was incomplete waited for a ruling on it — and nothing it wrote meanwhile was ever recorded.
func (a *Act) CmdRevoke(c registry.Caller, args []string, out io.Writer) (int, error) {
	ps := a.Store.For(c.Project)
	st, err := ps.GetState(c.Agent)
	if err != nil {
		return 1, err
	}
	pr, ok, err := a.livePR(c.Project, c.Agent)
	if err != nil {
		return 1, err
	}
	if !ok {
		fmt.Fprintln(out, prompts.ReplyNothingToRevoke)
		return 1, nil
	}
	reason := strings.TrimSpace(strings.Join(args, " "))
	if reason == "" {
		reason = "the author withdrew it"
	}
	// SCRAPPED, not rejected: a withdrawal is the author closing its own PR, where a rejection is a
	// verdict a resubmission clears — so "rejected" left it live (-> api.PROpen) and every later
	// reader handed the author back to it. sudri withdrew this one and was still being told to fix it.
	pr.Status, pr.Feedback = "scrapped", "withdrawn by "+c.Agent+": "+reason
	if err := ps.PutPR(pr); err != nil {
		return 1, err
	}
	// The branch submitting took it from, container and all, so a feature worker returns to its own
	// tree. Where that leaves it is its map's (-> cond.PRSettled).
	if err := ps.SetHolding(c.Agent, st.Task, pr.Branch, st.Container,
		store.ReasonAdvanced, "PR withdrawn, resuming work: "+pr.ID); err != nil {
		return 1, err
	}
	a.announceHolding(c.Project, c.Agent)
	// Whoever was reading it is reading a branch about to change under them.
	a.ReleaseReviewers(c.Project, pr.ID, "withdrawn by its author before a verdict")
	_ = ps.LogPR(pr.ID, "withdrawn", "by "+c.Agent+": "+reason)
	_ = ps.Log(c.Agent, "revoke", pr.ID+": "+reason)
	a.Deps.Notify()
	fmt.Fprintln(out, prompts.ReplyRevoked(pr.ID, st.Task))
	return 0, nil
}

// livePR finds the PR an agent has out that has not landed or been discarded.
func (a *Act) livePR(project, agent string) (store.PR, bool, error) {
	prs, err := a.Store.For(project).PRs()
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
func (a *Act) RejectPR(project, prID, feedback string) error {
	return a.reject(project, prID, feedback, api.SenderUser)
}

// reject routes feedback to the owning worker. voice is WHO ruled: "user", "reviewer", or a coauthor
// by name, which is also the author its badge carries.
func (a *Act) reject(project, prID, feedback, voice string) error {
	ps := a.Store.For(project)
	pr, ok, err := ps.GetPR(prID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w %q", core.ErrNoSuchPR, prID)
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
	// The work comes back, container and all: dropping it took a feature worker off its own branch on
	// a rejected milestone. Where the rejection leaves it is its map's (-> cond.Rejected).
	prior, _ := ps.GetState(pr.Agent)
	_ = ps.SetHolding(pr.Agent, pr.Task, pr.Branch, prior.Container,
		store.ReasonRejected, "rejected by "+voice+": "+prID)
	a.announceHolding(project, pr.Agent)

	who, msg := voice, prompts.MsgRejectedByAgent(voice, pr.ID)
	if voice == api.SenderUser {
		msg = prompts.MsgRejectedByUser(pr.ID)
	}
	if prior.Container != "" { // the milestone is the user's to re-open; there is nothing to re-submit
		msg = prompts.MsgMilestoneRejected(prior.Container, who)
	}
	_ = ps.LogPR(pr.ID, "rejected", "by "+who+": "+feedback)
	_ = ps.Log(pr.Agent, "reject", pr.ID+" ("+who+"): "+feedback)
	// The author's session is NOT cleared here: a new round starts fresh, and that is the author's
	// own map to do on its way to the feedback (-> worker's refreshing). A verdict writes a verdict.
	// From whoever ruled: an agent weights feedback by who it is from.
	_ = a.Harness.Say(project, pr.Agent, msg, mail.MailAndPush.From(who))
	a.Deps.Notify()
	return nil
}

// CmdReject is an agent's reject: a "changes" verdict in the voice of whoever gave it — the role for
// a reviewer, its own name for a coauthor, which speaks for nobody but itself.
func (a *Act) CmdReject(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(out, "usage: reject <pr-id> <feedback...>")
		return 2, nil
	}
	// callerPRProject, not PRProject: it widens beyond c.Project only when c itself holds this PR.
	prProject := a.callerPRProject(c, args[0])
	pr, ok, err := a.Store.For(prProject).GetPR(args[0])
	if err != nil {
		return 1, err
	}
	if ok && ownWork(pr, c) {
		fmt.Fprintln(out, prompts.ReplyNoSelfVerdict(pr.ID, "reject"))
		return 1, nil
	}
	feedback := strings.Join(args[1:], " ")
	if err := a.reject(prProject, args[0], feedback, rejectVoice(c)); errors.Is(err, core.ErrNoSuchPR) {
		fmt.Fprintln(out, prompts.ReplyNoSuchPR(args[0]))
		return 1, nil
	} else if err != nil {
		return 1, err
	}
	if c.Role != "coauthor" { // its badge is reject's own, written under its name (-> reject)
		a.completeReview(prProject, c.Project, args[0], c.Agent, "changes", strings.TrimSpace(feedback))
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
