// package: hub/workflow / injected
// type:    logic (the [hub]/[user]/[reviewer] lines typed into an agent's session)
// job:     what the hub SAYS to an agent unprompted — a verdict, a merge, a task
// cancelled under it, a reference branch that moved. Unlike a directive,
// which answers `sindri`, these arrive whether or not the agent asked.
// limits:  the strings only; when each is sent is the workflow's, and delivery is
// the hub's (-> InjectWhenReady).
package workflow

import (
	"fmt"
	"time"
)

const MsgKickoff = "[hub] You're live. Run `sindri` and do exactly what it tells you — it always returns your current job, whether you're new or resuming."

// MsgWorkAvailable nudges an idle worker that rated work exists. Claiming stays a pull, so two
// workers can't take one task — but an agent that stopped asking would never hear about it.
func MsgWorkAvailable(id string) string {
	return fmt.Sprintf("[hub] New work is ready (%s). Run `sindri` to pick up your next task — it may not be this one, whichever is highest priority.", id)
}

// MsgStalled prods an agent that holds work but has gone quiet. It names the task, because a stalled
// agent has usually lost the thread rather than the will, and it offers the other honest answer —
// saying what blocks it — so a genuine blocker surfaces instead of being sat on.
func MsgStalled(task string, idleFor time.Duration) string {
	return fmt.Sprintf("[hub] You still hold %s and have been idle for %s. Carry on with it — run `sindri` "+
		"if you need your directive again. If something blocks you, say what it is rather than waiting: "+
		"nothing is coming unless you ask.", task, idleFor.Round(time.Minute))
}

// MsgRetryTurn restarts a turn the API cut off. It names the cause, because the agent's own last
// output is truncated and it would otherwise reason from a half-finished thought as if it were
// complete — and it says to re-check the work rather than assume the interrupted step landed.
const MsgRetryTurn = "[hub] Your last response was cut off mid-stream by an API error, so nothing " +
	"resumed on its own. Pick up where you left off: check whether the step you were on actually " +
	"completed (`sindri git change` shows what is written) before carrying on, since your own last " +
	"message is truncated and may describe work that never happened."

// MsgMerged tells a worker its PR merged and to fetch the next task.
func MsgMerged(prID string) string {
	return fmt.Sprintf("[hub] %s merged. Run `sindri` for your next task.", prID)
}

// MsgPRScrapped tells an author its PR was discarded. Deliberately final — the branch is gone, so
// unlike a rejection there is nothing to resubmit. Without it the author waits in "submitted".
func MsgPRScrapped(prID string) string {
	return fmt.Sprintf("[user] %s was scrapped — the work isn't wanted and its branch is gone. "+
		"Nothing to fix or resubmit. Run `sindri` for your next directive.", prID)
}

// MsgTaskEdited tells a worker the task it holds was revised under it. Its unit of work has moved,
// and it is otherwise still building to the version it read when it picked the task up — so this
// names the fields and sends it back to the task, where the old and new values are recorded in
// full. It also says the obvious thing that stops being obvious once the task shows "pending"
// again: the work is still the worker's to finish, and it finishes it the same way.
func MsgTaskEdited(id, fields string) string {
	return fmt.Sprintf("[hub] A planner edited %s — the task you're working on (%s). Read it again "+
		"(`sindri task %s`, where the change is recorded on its thread) and work to what it says now, "+
		"not to what you read when you picked it up. It is back awaiting the user's approval, which "+
		"only holds it from being handed out afresh: it is still yours, and you finish and submit it "+
		"exactly as before.", id, fields, id)
}

// MsgTaskCancelled tells a worker its task was closed/scrapped out from under it —
// stop, and don't clean up (the hub already reset the worktree), just get new work.
func MsgTaskCancelled(id string) string {
	return fmt.Sprintf("[hub] Task %s was cancelled — stop working on it. Don't clean up your workspace; the sindri hub will reset it for you when you pick up your next task. Just run `sindri`.", id)
}

// MsgReviewCancelled tells a reviewer the PR it was reviewing was scrapped, so the
// review is moot — stop and pick up new work. Its branch is gone, so there's nothing
// left to read.
func MsgReviewCancelled(prID string) string {
	return fmt.Sprintf("[hub] The PR you were reviewing (%s) was scrapped — stop reviewing it; its branch is gone. Just run `sindri` for your next task.", prID)
}

// MsgVerdictRecorded puts a reviewer back in the loop right after a verdict, its reply having
// named no next step.
func MsgVerdictRecorded(prID string) string {
	return fmt.Sprintf("[hub] Verdict on %s recorded. Run `sindri` for your next review.", prID)
}

// ReplyNothingToRevoke answers `revoke` with no PR out — nothing was withdrawn, so it says what the
// agent's actual situation is rather than reporting a success that did not happen.
const ReplyNothingToRevoke = "Nothing to withdraw — you have no pull request out. Run `sindri` for your current directive."

// ReplyRevoked confirms a withdrawal and says where it leaves the agent: back on the same branch,
// with the work it had already submitted still on it.
func ReplyRevoked(prID, task string) string {
	return fmt.Sprintf("Withdrew %s — it will not be merged, and %s is yours again. Your branch is "+
		"untouched, so everything you had submitted is still on it: carry on, then `sindri submit "+
		"\"<summary>\"` when it is genuinely done.", prID, task)
}

// MsgReviewAmended adds instructions to a review already under way. The same PR and the same branch
// — so it says to carry on rather than restart, and that the verdict now answers both.
func MsgReviewAmended(prID, requirement string) string {
	return fmt.Sprintf("[user] More to check on %s, which you're already reviewing — same PR, same "+
		"branch in /workspace, carry on from where you are and let your verdict cover this too:\n\n%s",
		prID, requirement)
}

// MsgRebased tells a worker the hub rebased its branch onto a moved base.
func MsgRebased(base string) string {
	return fmt.Sprintf("[hub] %s moved — your branch was rebased onto it, so you're up to date.", base)
}

// MsgReferenceAdvanced tells an agent the reference gained commits and the hub replayed its work
// onto them, naming what arrived so it can re-check anything of its own that builds on it.
func MsgReferenceAdvanced(incoming []string) string {
	return fmt.Sprintf("[hub] %s moved on and your work was rebased onto it — you're aligned, and nothing of yours was lost.%s\nIf any of your work builds on what changed, check it before carrying on (`sindri git change` shows yours).", refName, commitList(incoming))
}

// MsgReferenceNeedsRebase is the same movement when the hub could NOT replay the agent's work —
// usually its own uncommitted edits. It says what is waiting and leaves the move to the agent.
func MsgReferenceNeedsRebase(incoming []string) string {
	return fmt.Sprintf("[hub] %s moved on, but your work couldn't be rebased onto it automatically — most often because of uncommitted edits in /workspace.%s\nRun `sindri rebase` when you're at a clean point; it will tell you about any conflicts to fix.", refName, commitList(incoming))
}

// MsgReferenceRewritten is the dangerous case: the reference's history was REPLACED, so conclusions
// an agent drew about code it doesn't own may describe commits that no longer exist.
func MsgReferenceRewritten() string {
	return fmt.Sprintf("[hub] %s was REWRITTEN — its history was replaced, not just extended. Your own commits are intact and nothing of yours was discarded, but anything you concluded about code you don't own may now be out of date, including gate failures you attributed to other people's files. Re-check such a finding against the current tree before acting on it or arguing from it. Run `sindri rebase` when you're at a clean point, then `sindri git incoming` to see where you stand.", refName)
}

// MsgResolveNeeded is injected when a branch can't merge because it conflicts with
// its base: the hub has left the conflicts in the worker's workspace to edit (the
// worker has no git — the hub drives it), and points at the single verb to retry.
func MsgResolveNeeded(base string, files []string) string {
	return fmt.Sprintf("[hub] Your branch conflicts with %s and can't be merged yet: %s. The conflicts are in your /workspace with <<<<<<< markers — edit each file to the intended result (remove the markers), then run `sindri resolve`. Repeat until it's clean; it then goes back for review.", base, FileList(files))
}

// MsgMilestoneMerged tells a feature worker its milestone merged and its branch was
// rebased onto the new base.
func MsgMilestoneMerged(prID string) string {
	return fmt.Sprintf("[hub] Milestone %s merged — your feature branch is rebased onto the new base. Run `sindri` to continue.", prID)
}

// MsgMilestoneRejected is the rejection a feature worker gets. It names the feature rather than the
// subtask the worker happens to be holding, since the PR covers the whole branch. voice is who ruled
// ("user" or "reviewer").
func MsgMilestoneRejected(container, voice, feedback string) string {
	return fmt.Sprintf("[%s] The PR for feature %s was rejected: %s — address it on the branch you're "+
		"already on, then `sindri submit \"<summary>\"` to put the feature up again.",
		voice, container, feedback)
}

// MsgRejectedByUser tells a worker the user rejected its PR, with the feedback.
func MsgRejectedByUser(prID, feedback string) string {
	return fmt.Sprintf("[user] %s was rejected: %s — address the feedback on your branch and run `sindri submit` again.", prID, feedback)
}

// MsgRejectedByReviewer tells a worker its reviewer rejected the PR, with the feedback.
func MsgRejectedByReviewer(prID, feedback string) string {
	return fmt.Sprintf("[reviewer] %s rejected: %s — please address the feedback and submit again.", prID, feedback)
}

// MsgReview is the single review instruction: the hub has already checked the PR branch out into
// the reviewer's /workspace, so it points there. A failed checkout says so loudly and falls back
// to the socket diff, so a stale tree is never mistaken for the PR.
func MsgReview(prID, requirement, branch, base, arch string, checkedOut bool) string {
	seeChanges := fmt.Sprintf("`sindri show %s`", prID)
	loc := ""
	if checkedOut {
		// /workspace is a linked worktree with no reachable .git, so `git diff` fails in the
		// pod — the checkout is for READING the code in context; the diff comes from the hub.
		loc = fmt.Sprintf("PR branch %s is checked out FRESH in /workspace, based on %s. ", branch, base)
	} else {
		// Loud: the checkout failed, so /workspace is NOT this PR. Say so plainly
		// rather than letting the reviewer assume /workspace holds the change.
		loc = fmt.Sprintf("⚠ %s could NOT be checked out into /workspace — review from the diff only; do NOT trust /workspace. ", branch)
	}
	return fmt.Sprintf("[hub] Review %s — %s %s(1) see what changed: %s. (2) check the gate: `sindri lint %s`. (3) decide: `sindri approve %s` or `sindri reject %s \"<findings>\"`.%s",
		prID, requirement, loc, seeChanges, prID, prID, prID, ReviewArchitecture(arch))
}
