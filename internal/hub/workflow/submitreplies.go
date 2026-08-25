// package: hub/workflow / submitreplies
// type:    logic (agent-facing strings for the submit/contribute/rebase lifecycle)
// job:     everything an agent is told about putting work up, waiting on its gate,
// rebasing, and resolving conflicts — split out of prompts.go, which was doing
// too many jobs at once.
// limits:  pure strings/builders; which one to use is the workflow's.
package workflow

import "fmt"

const DirSubmitted = "Your pull request is under review. Wait — the hub will tell you the verdict. " +
	"While you wait, `sindri resolve` checks your branch still merges onto its base (and resolves it " +
	"if the base has moved); it does no harm and keeps the PR healthy. And if you realise the work " +
	"is NOT finished after all, don't sit on it: `sindri revoke \"<why>\"` withdraws the PR and hands " +
	"the task back to you on the same branch, so you can finish it and submit again."

// DirGating answers a worker whose submit/contribute is queued for its quality gate — there is no
// PR yet, so DirSubmitted's wording would claim one exists. A failure reaches it as a message
// either way; a pass moves it straight to review WITHOUT one, since nothing about that changes what
// it does next — asking again is how it would notice, not required to.
const DirGating = "Your quality gate is queued. A failure reaches you as a message, with what to " +
	"fix. A pass sends nothing — you're simply moved to review; ask `sindri` again if you want to see it."

// ReplyRegistered acknowledges a submitted PR and tells the worker to wait for review.
func ReplyRegistered(prID string) string {
	return fmt.Sprintf("%s registered. You'll be informed when it's reviewed. Please wait — this may take a while.", prID)
}

// ReplyGateQueued answers submit/contribute at once: the gate is queued, not run yet, so there is
// no PR to name — position is the same fact `sindri run` reports. Same asymmetry as DirGating: a
// failure is worth a message, a pass isn't.
func ReplyGateQueued(runID string, position int) string {
	return fmt.Sprintf("Quality gate %s queued at position %d. A failure reaches you as a message; a pass moves you to review without one.", runID, position)
}

// ReplyGateReused answers a landing verb whose commit already has a passing verdict: nothing ran, so
// the continuation has ALREADY happened by the time this is printed — which is why it points at the
// message rather than restating it. A reused pass that read like a fresh one would hide both facts.
func ReplyGateReused(runID, sha string) string {
	return fmt.Sprintf("Quality gate %s did not need to run: %s already passed and nothing has changed "+
		"since, so the result stands. What follows it has already been sent to you — read that, not this.", runID, sha)
}

// ReplyLintQueued answers `lint` with no stored verdict for the commit: the gate builds and tests, so
// it goes through the fleet's single slot like every other one rather than running N at a time.
func ReplyLintQueued(runID, sha string, position int) string {
	return fmt.Sprintf("Quality gate %s queued at position %d, on your work as the hub recorded it (%s). You'll "+
		"be told the result — carry on with something else; a position is not a failure, so don't retry.", runID, position, sha)
}

// ReplyReviewRequestFailed tells a submitting agent its PR is up but requesting a review failed
// (-> RepairReviewRows retries it in the background).
func ReplyReviewRequestFailed(prID string, err error) string {
	return fmt.Sprintf("%s registered, but requesting a review failed: %v. The hub retries this on its own; flag it if %s is still showing no reviewer after a while.", prID, err, prID)
}

// ReplyNoSuchPR answers an id no PR carries, and names the listing that does — an unknown id is a
// dead end, so the reply's content is where to look instead. An ordinary answer: reported as an
// error it reaches AgentExec as a hub fault and stops the agent (-> ErrNoSuchPR).
func ReplyNoSuchPR(id string) string {
	return fmt.Sprintf("No PR %s here. `sindri prs` lists the ones you can act on; a pooled reviewer sees "+
		"the PR it was handed. Check the id there, then carry on with `sindri`.", id)
}

// ReplyPRStillToLand refuses to end a task whose PR has not merged. Names the way out, since a
// rejected author is in the state that most looks finished and is not.
func ReplyPRStillToLand(task, pr string) string {
	return fmt.Sprintf("Not closing %s: %s has not merged, so the task is still yours. If it was rejected, "+
		"answer the feedback and `sindri submit` again — a task ends when its PR lands, never before.", task, pr)
}

// ReplySubmitTaskClosed refuses a submission whose target closed under it. It states the fact and
// stops there: nothing the agent can fix, and nothing gained by gating work with nowhere to land.
func ReplySubmitTaskClosed(task string) string {
	return fmt.Sprintf("Not submitted: task %s is closed, so there is nothing for a pull request to land into. "+
		"Your work is committed and your branch keeps it. Do not submit again — if what is on it is still "+
		"wanted, `sindri escalate` says so; otherwise run `sindri` for new work.", task)
}

// ReplyNotWorking guards a work verb run in a phase it doesn't apply to. It must name the ACTUAL
// state: a flat "pick up a task first" told a worker under review to abandon the task it held.
func ReplyNotWorking(verb, phase, task string) string {
	switch {
	case task == "" || phase == "idle":
		return fmt.Sprintf("Nothing to %s — you have no task. Run `sindri` to pick one up.", verb)
	case phase == "submitted":
		return fmt.Sprintf("Can't %s %s — its PR is under review. Wait for the verdict.", verb, task)
	case phase == "resolving":
		return fmt.Sprintf("Can't %s %s while resolving. Fix the <<<<<<< markers in /workspace, then call `sindri resolve`.", verb, task)
	case phase == "gating":
		return fmt.Sprintf("Can't %s %s — its quality gate is queued. Wait for the result.", verb, task)
	}
	return fmt.Sprintf("Can't %s %s from phase %q. Run `sindri` for your directive.", verb, task, phase)
}

// ReplyContributed confirms an interim contribution is recorded and gated on the
// user's approval — the worker then waits until it's merged (and told to continue).
func ReplyContributed(prID string) string {
	return fmt.Sprintf("Interim contribution %s recorded — it needs the user's approval before it merges into the reference branch. Wait; you'll be told to keep going once it lands. (This may take a while.)", prID)
}

// ReplyMilestoneContributed confirms a feature branch is up as it stands. It names the FEATURE,
// since that is what the PR contains — a worker told its subtask went up would misread what landed.
func ReplyMilestoneContributed(prID, feature string) string {
	return fmt.Sprintf("Feature %s is up as %s — everything recorded on the branch so far, waiting on the user to merge it. Wait; you'll be told to carry on with the next subtask once it lands. (This may take a while.)", feature, prID)
}

// ReplyContributeConflicts tells a worker its contribution doesn't rebase cleanly yet — fix the
// markers and run `sindri resolve`, which finishes the interim PR once clean.
func ReplyContributeConflicts(base string, files []string) string {
	return fmt.Sprintf("Your contribution doesn't rebase cleanly onto %s yet — conflicts in %s. Fix the <<<<<<< markers in /workspace, then run `sindri resolve`; once clean the contribution awaits the user's approval.", base, FileList(files))
}

// ReplyContributionClean confirms a resolved interim contribution now applies cleanly
// and is waiting for the user (no reviewer — interim PRs are user-gated).
func ReplyContributionClean(base string) string {
	return fmt.Sprintf("Your contribution now applies cleanly onto %s — it awaits the user's approval. You'll be told to keep going once it merges.", base)
}

// MsgContributionMerged tells a worker its interim contribution landed (branch fast-forwarded)
// and to keep working the SAME task, which stays open.
func MsgContributionMerged(prID, task string) string {
	return fmt.Sprintf("[hub] Your interim contribution %s merged into the reference branch and your branch was fast-forwarded past it — keep working on task %s. Run `sindri contribute` again to land more, or `sindri submit` when the task is done.", prID, task)
}

// ReplyRebaseConflicts answers `rebase` when the rebase hit conflicts to edit.
func ReplyRebaseConflicts(files []string) string {
	return fmt.Sprintf("Rebasing onto %s hit conflicts in %s. They're in your /workspace with <<<<<<< markers — edit each file to the intended result (remove the markers), then run `sindri rebase` again to continue. Repeat until it reports you're aligned.", refName, FileList(files))
}

// ReplyRebaseStashConflicts answers `rebase` when the commits rebased but the worker's uncommitted
// edits then clashed. Says which, so it resolves those edits without doubting its commits.
func ReplyRebaseStashConflicts(files []string) string {
	return fmt.Sprintf("Your recorded work is rebased onto %s — only your loose edits to %s clash with it. They're in your /workspace with <<<<<<< markers — edit each file to the intended result (remove the markers), then run `sindri rebase` again to finish. Nothing is lost, and none of the work you've already handed over is in question.", refName, FileList(files))
}

// ReplyRebased answers `rebase` once the branch is current, listing what came in: those commits
// changed the code under the agent unseen, and only `rebase` is placed to say what they were.
func ReplyRebased(incoming []string) string {
	s := fmt.Sprintf("Your branch is rebased onto %s — you're aligned with the current reference state.", refName)
	if len(incoming) == 0 {
		return s + " Nothing new came in. Carry on."
	}
	s += fmt.Sprintf("\n\nIt brought in %d commit(s), which changed the code under you:\n", len(incoming))
	for _, l := range incoming {
		s += "  " + l + "\n"
	}
	return s + "\nCheck anything of yours that builds on them (`sindri git change` shows your own change). Carry on."
}

// ReplyResolveDirty answers `resolve` on a dirty worktree, suggesting nothing git-based: the pod
// doesn't mount the real .git, so every git command fails. The verb it names tracks the caller's
// surface — contribute/submit exist only in "working", and a feature worker holds checkpoint.
func ReplyResolveDirty(phase string, inContainer bool) string {
	const dirty = "Changes in /workspace the hub hasn't recorded yet block the rebase. "
	switch phase {
	case "working":
		if inContainer {
			return dirty + "Call `sindri checkpoint \"<summary>\"` for the hub to record them and move to your next subtask."
		}
		return dirty + "Call `sindri contribute \"<summary>\"` for the hub to record them and rebase (the task stays open), or `sindri submit \"<summary>\"` if the task is done."
	case "submitted":
		return dirty + "Your PR is under review — leave them and wait for the verdict. Note them with `sindri log \"<note>\"`."
	case "gating":
		return dirty + "Your quality gate is queued — leave them and wait for the result. Note them with `sindri log \"<note>\"`."
	}
	return dirty + "Run `sindri` for your directive."
}

// ReplyResolveConflicts answers `resolve` when conflicts remain to edit.
func ReplyResolveConflicts(base string, files []string) string {
	return fmt.Sprintf("Rebasing onto %s conflicts in %s. They're in your /workspace with <<<<<<< markers — edit each file to the intended result (remove the markers), then run `sindri resolve` again.", base, FileList(files))
}

// ReplyResolvedClean answers `resolve` once the branch applies cleanly after a
// conflict was resolved — it's back with the reviewer.
func ReplyResolvedClean(base string) string {
	return fmt.Sprintf("Your branch is now current with %s and conflict-free — it's back with the reviewer.", base)
}

// ReplyAlreadyCurrent answers a proactive `resolve` on a branch that already sits
// cleanly on its base.
func ReplyAlreadyCurrent(base string) string {
	return fmt.Sprintf("Your branch is already current with %s — nothing to resolve.", base)
}

// ReplyReapplyResolved answers `resolve` once a milestone's post-merge reapply conflict is
// cleared. Unlike ReplyResolvedClean, nothing goes back to a reviewer — the merge already landed.
func ReplyReapplyResolved() string {
	return "Resolved. The merge already landed, so there's nothing to resubmit — run `sindri` to carry on."
}

// ReplyTaskProposed acknowledges a planner's proposed task, pending user approval. nudge is the
// unparented-siblings reminder (-> unparentedNudge), empty when there's nothing recent to name.
func ReplyTaskProposed(id, title, nudge string) string {
	return fmt.Sprintf("Proposed %s: %s — awaiting the user's approval before any worker can pick it up.%s", id, title, nudge)
}

// ReplyBehindBase refuses a submit whose branch the reference has moved past, naming how far behind
// and what arrived — the commits are what tell an agent whether its work still makes sense.
func ReplyBehindBase(base string, behind int, incoming []string) string {
	return fmt.Sprintf("Not submitted: your branch is %d commit(s) behind %s, so the PR would be "+
		"reviewed and merged against a base that has moved.\n"+
		"Run `sindri rebase` (it resolves conflicts step by step if there are any), then `sindri "+
		"submit` again — the quality gate re-runs on the rebased tree, so what you put up is "+
		"verified against the state it will actually merge into.%s",
		behind, base, commitList(incoming))
}

// ReplyGateFail echoes what the gate found and states the one rule agents keep discovering the
// expensive way: it passes only if EVERYTHING passes. One resubmitted the same failing test three
// times, having correctly judged it pre-existing — a fact the gate cannot act on and never claimed to.
func ReplyGateFail(out string) string {
	return fmt.Sprintf("The quality gate FAILED, so no PR was created:\n%s\n%s", out, gateRule)
}

// gateRule is why a resubmission of the same tree is wasted: the gate reports what is broken, never
// who broke it, so "pre-existing" and "unrelated" change nothing about the verdict. Escalation is the
// way out when a fault genuinely belongs outside the task — nothing else here is.
const gateRule = "The gate passes only if EVERYTHING passes. It does not ask who caused a failure " +
	"and cannot: pre-existing, unrelated, somebody else's — the verdict is the same, and every line " +
	"in this repo was written by an agent, so there is nobody else to hand it to. Submitting the same " +
	"tree again returns this same result and spends the fleet's one run slot doing it. Two ways " +
	"forward: fix it, whoever wrote it; or `sindri escalate \"<what needs deciding>\"` if it truly " +
	"belongs outside your task and the user must rule on it."

// ReplySpecInvalid answers `openspec submit` when the change fails openspec's own
// validation (the planner's gate — the code linter doesn't apply to spec work).
func ReplySpecInvalid(out string) string {
	return fmt.Sprintf("openspec validation failed — fix the specs and submit again:\n%s", out)
}
