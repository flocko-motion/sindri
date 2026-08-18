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
// PR yet, so DirSubmitted's wording would claim one exists. Wait only.
const DirGating = "Your quality gate is queued — wait for the result, which arrives as a message " +
	"(a PR if it passes, feedback to fix if it fails). Nothing to check in the meantime."

// ReplyRegistered acknowledges a submitted PR and tells the worker to wait for review.
func ReplyRegistered(prID string) string {
	return fmt.Sprintf("%s registered. You'll be informed when it's reviewed. Please wait — this may take a while.", prID)
}

// ReplyGateQueued answers submit/contribute at once: the gate is queued, not run yet, so there is
// no PR to name — position is the same fact `sindri run` reports.
func ReplyGateQueued(runID string, position int) string {
	return fmt.Sprintf("Quality gate %s queued at position %d. You'll be told the result — no need to ask again.", runID, position)
}

// ReplyReviewRequestFailed tells a submitting agent its PR is up but requesting a review failed
// (-> RepairReviewRows retries it in the background).
func ReplyReviewRequestFailed(prID string, err error) string {
	return fmt.Sprintf("%s registered, but requesting a review failed: %v. The hub retries this on its own; flag it if %s is still showing no reviewer after a while.", prID, err, prID)
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

// ReplyTaskProposed acknowledges a planner's proposed task, pending user approval.
func ReplyTaskProposed(id, title string) string {
	return fmt.Sprintf("Proposed %s: %s — awaiting the user's approval before any worker can pick it up.", id, title)
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

// ReplyLintFail echoes the violations, and says a finding is to be MET, not evaded: relocating
// prose or widening a limit clears the report while leaving the problem the rule exists for.
func ReplyLintFail(out string) string {
	return fmt.Sprintf("Lint failed — fix the violations and submit again:\n%s\n"+
		"Meet each finding on its own terms; do NOT work around the linter. If a comment is "+
		"too long, CUT WORDS — don't move it somewhere the rule doesn't reach, don't split it "+
		"or pad the file with one-liners to shift an average, don't widen an ignore list, and "+
		"don't retune limits in .sindri/config.yaml (those are the maintainer's call).\n"+
		"Every limit is a CEILING, not a target. Don't trim until the number just passes — "+
		"trim until the comment earns its lines. A single line suffices for most: say what the "+
		"thing is for, or why it isn't done the obvious way, and stop. Land well under the "+
		"limit, or the next comment anyone adds puts the file straight back over it.", out)
}

// ReplySpecInvalid answers `openspec submit` when the change fails openspec's own
// validation (the planner's gate — the code linter doesn't apply to spec work).
func ReplySpecInvalid(out string) string {
	return fmt.Sprintf("openspec validation failed — fix the specs and submit again:\n%s", out)
}
