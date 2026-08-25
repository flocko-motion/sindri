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

// MsgUnretired tells a retired agent it is back in service. DirRetired sends it away from asking
// again on its own, so this push is the only thing that would ever reach it (-> Hub.SetRetired).
const MsgUnretired = "[hub] You're back in service — the user has un-retired you. Run `sindri` for your next action."

// MsgWorkAvailable nudges an idle worker with what IT would be handed right now — nudgeIdleWorkers
// computed id against this agent's own preferences, not just the task that triggered the check.
// Named, but not guaranteed: claiming stays a pull, so another agent asking first can still take it.
func MsgWorkAvailable(id string) string {
	return fmt.Sprintf("[hub] %s is ready for you. Run `sindri` to claim it — someone else may beat you to it, in which case you'll be handed whatever is next.", id)
}

// MsgStalled prods an agent that holds work but has gone quiet, naming the task and inviting it
// to say what blocks it — so a genuine blocker surfaces instead of being sat on.
func MsgStalled(task string, idleFor time.Duration) string {
	return fmt.Sprintf("[hub] You still hold %s and have been idle for %s. Carry on with it — run `sindri` "+
		"if you need your directive again. If something blocks you, say what it is rather than waiting: "+
		"nothing is coming unless you ask.", task, idleFor.Round(time.Minute))
}

// MsgRetryTurn restarts a turn the API cut off, naming the cause — the agent's own last output is
// truncated, and it says to re-check the work rather than assume the interrupted step landed.
const MsgRetryTurn = "[hub] Your last response was cut off mid-stream by an API error, so nothing " +
	"resumed on its own. Pick up where you left off: check whether the step you were on actually " +
	"completed (`sindri git change` shows what is written) before carrying on, since your own last " +
	"message is truncated and may describe work that never happened."

// MsgPRScrapped tells an author its PR was discarded. Deliberately final — the branch is gone, so
// unlike a rejection there is nothing to resubmit. Without it the author waits in "submitted".
func MsgPRScrapped(prID string) string {
	return fmt.Sprintf("[user] %s was scrapped — the work isn't wanted and its branch is gone. "+
		"Nothing to fix or resubmit. Run `sindri` for your next directive.", prID)
}

// MsgTaskEdited tells a worker a task inside its unit was revised under it — its own, or one
// anywhere inside a held feature — naming which, and what "pending" does and doesn't mean for it.
func MsgTaskEdited(id, unit, fields string) string {
	if id == unit {
		return fmt.Sprintf("[hub] A planner edited %s — the task you're working on (%s). Read it again "+
			"(`sindri task %s`, where the change is recorded on its thread) and work to what it says now, "+
			"not to what you read when you picked it up. It is back awaiting the user's approval, which "+
			"only holds it from being handed out afresh: it is still yours, and you finish and submit it "+
			"exactly as before.", id, fields, id)
	}
	return fmt.Sprintf("[hub] A planner edited %s (%s) — not the task you're on, but one inside %s, "+
		"the work you hold. Read it (`sindri task %s`, where the change is recorded on its thread) and "+
		"check whether what you are building still fits it. It is back awaiting the user's approval, so "+
		"it cannot be handed to you until they rule — and %s is not finished while it waits, so carry "+
		"on with what you have in hand.", id, fields, unit, id, unit)
}

// MsgTaskGainedChild tells an agent its unit of work grew: a child was added under a task it holds.
// promoted says whether that turned a leaf into a feature, changing what it runs next. The bar for
// its PR rose either way, and being refused at a checkpoint is not how it should find that out.
func MsgTaskGainedChild(parent, child string, promoted bool) string {
	if !promoted {
		return fmt.Sprintf("[hub] %s was added under %s, which is inside the work you hold. Read it "+
			"(`sindri task %s`) — it is part of your feature now, so its PR waits for this too. Carry "+
			"on with the subtask you are on; the hub hands you this one in its turn.", child, parent, child)
	}
	return fmt.Sprintf("[hub] %s gained a child, %s, so what you hold is now a FEATURE rather than a "+
		"single task — same branch, nothing of yours lost. You take the new work on too, and it goes "+
		"up as ONE pull request covering the whole of it. Run `sindri` for the subtask to work, "+
		"`sindri checkpoint \"<summary>\"` to end each one, and `sindri submit \"<summary>\"` only "+
		"when none are left. Read the new work first: `sindri task %s`.", parent, child, child)
}

// MsgTaskCancelled tells a worker its task was closed/scrapped out from under it —
// stop, and don't clean up (the hub already reset the worktree), just get new work.
func MsgTaskCancelled(id string) string {
	return fmt.Sprintf("[hub] Task %s was cancelled — stop working on it. Don't clean up your workspace; the sindri hub will reset it for you when you pick up your next task. Just run `sindri`.", id)
}

// MsgSubmitTaskClosed tells an author its submission found no task to land into. A fact about the
// world, never a violation to fix, so it does not ask for a resubmission — that would loop.
func MsgSubmitTaskClosed(task string) string {
	return fmt.Sprintf("[hub] Nothing to submit into: task %s closed while your gate was running, so no pull request was opened. Your work is committed and your branch is untouched. Do not submit again — if what you built is still wanted, say so with `sindri escalate`, otherwise run `sindri` for new work.", task)
}

// MsgHierarchyTaken tells a worker its feature went to whoever is already inside it. Names the other
// agent, since "you no longer hold it" without a reason reads as work being taken away.
func MsgHierarchyTaken(container, other string) string {
	return fmt.Sprintf("[hub] Feature %s is released: %s is working inside it, and one tree is one agent's — "+
		"two would put two branches on the same subtasks. Your own work on it is untouched and %s carries on "+
		"from here. Run `sindri` for your next task.", container, other, other)
}

// MsgNoGateQuestion is what the USER reads on the escalation list, so it states the decision rather
// than the incident: one line, naming the thing to set.
const MsgNoGateQuestion = "This project has no quality gate: `verify:` is unset in .sindri/config.yaml, so " +
	"nothing can be submitted here until it names the command that builds, tests and lints — `verify: make check`."

// MsgNoGateEscalated tells the agent it has been stopped and where the cause lies. Carries the
// gate's own words, since they name the fix — the user may well ask the agent to make it.
func MsgNoGateEscalated(detail string) string {
	return "[hub] You are now ESCALATED and your submission did not go through: this project declares no " +
		"quality gate, which only the user can set. Your work is fine and your diff is beside the point — " +
		"the fault is in the project's configuration, so leave the code alone and stop here.\n\n" + detail +
		"\nThe user has been asked. Wait for their answer; if they ask you to write the gate script, that " +
		"becomes your work."
}

// MsgPRSettledWithTask tells an author its PR was rejected because its task closed. Says the work
// was not judged — the author is the one who knows whether the branch still holds something wanted,
// and a rejection with no reading of the code reads as one otherwise.
func MsgPRSettledWithTask(prID, task string) string {
	return fmt.Sprintf("[hub] %s was rejected: its task %s is closed, so there is nothing left for it to land into. This is not a verdict on your work — nobody read it, and your branch still holds it. Do not resubmit; if what is on it is still wanted, say so with `sindri escalate`, otherwise run `sindri` for new work.", prID, task)
}

// MsgReviewCancelled tells a reviewer its PR was scrapped — stop, its branch is gone, get new work.
func MsgReviewCancelled(prID string) string {
	return fmt.Sprintf("[hub] The PR you were reviewing (%s) was scrapped — stop reviewing it; its branch is gone. Just run `sindri` for your next task.", prID)
}

// MsgRunFinished is what a run's scheduling agent is told — a summary, never the full log:
// injecting the whole output is how a 900k-token session happens, and one already has. The full
// log stays on the run record, fetched with `show <run-id>`.
func MsgRunFinished(id, status string, elapsed, budget time.Duration) string {
	verb := status
	if status == "timed_out" {
		verb = "timed out"
	}
	usage := ""
	if budget > 0 {
		usage = fmt.Sprintf(" (%s of its %s budget)", elapsed.Round(time.Second), budget.Round(time.Second))
	}
	return fmt.Sprintf("[hub] %s %s%s. Full output: `sindri show %s`.", id, verb, usage, id)
}

// MsgLintPassed answers a queued self-check that passed. It names the run, because the report on it
// names what was checked — the same result a submit reuses if the agent changes nothing after this.
func MsgLintPassed(runID string) string {
	return fmt.Sprintf("[hub] Your quality gate passed (`sindri show %s` for the report). Submitting "+
		"without changing anything reuses this result rather than gating again.", runID)
}

// MsgPRGateFinished tells a reviewer its PR check has landed. The verdict is on the PR and in the
// run's output rather than in here: a whole gate log injected into a session is how contexts die.
func MsgPRGateFinished(prID, runID string) string {
	return fmt.Sprintf("[hub] The quality gate on %s has finished — `sindri show %s` has the whole log, "+
		"and `sindri lint %s` the verdict alone. Neither re-runs it. Then give your verdict.", prID, runID, prID)
}

// MsgLintIncomplete answers a self-check that never reached a verdict. Unlike a landing gate's, there
// is nothing to re-submit: the agent kept its task the whole time and simply has no answer yet.
func MsgLintIncomplete(status string) string {
	word := status
	if status == "timed_out" {
		word = "timed out"
	}
	return fmt.Sprintf("[hub] Your quality gate did not complete (%s) — this says nothing about your "+
		"code. Run `sindri lint` again when you want the answer.", word)
}

// MsgGateFailed reuses ReplyGateFail's rulebook — only the delivery differs.
func MsgGateFailed(output string) string {
	return "[hub] " + ReplyGateFail(output)
}

// MsgGateIncomplete answers a gate that never reached a verdict (timeout, or a hub restart) —
// never as a violation, since nothing here found the code wrong.
func MsgGateIncomplete(status string) string {
	word := status
	if status == "timed_out" {
		word = "timed out"
	}
	return fmt.Sprintf("[hub] Your quality gate did not complete (%s) — this says nothing about your code. Run `sindri submit \"<summary>\"` (or `contribute`) again.", word)
}

// ReplyNoSelfVerdict refuses a verdict on the caller's own commits (05-workflow: no agent approves
// its own work). It says which half of the rule this is, since the other half is deliberately
// allowed: a coauthor may rule on a PR built from a task it wrote, and its badge names it.
func ReplyNoSelfVerdict(prID, verb string) string {
	return fmt.Sprintf("%s is built from your own commits, so its verdict is somebody else's to give "+
		"— you cannot %s it. (A task you WROTE is different: you may rule on work built from your "+
		"plan, and the badge records that it was you.)", prID, verb)
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
// reset onto the new base.
func MsgMilestoneMerged(prID string) string {
	return fmt.Sprintf("[hub] Milestone %s merged — your feature branch is reset onto the new base. Run `sindri` to continue.", prID)
}

// MsgReapplyConflict tells a worker its PR merged (milestone or plain interim contribution), but
// resetting its branch onto the new base couldn't reapply its own uncommitted work cleanly. Not
// "resolve needed": nothing failed to merge, so unlike MsgResolveNeeded no review awaits it.
func MsgReapplyConflict(prID, base string, files []string) string {
	return fmt.Sprintf("[hub] %s merged, but your uncommitted work didn't reapply cleanly onto %s: %s. The conflicts are in your /workspace with <<<<<<< markers — edit each file to the intended result (remove the markers), then run `sindri resolve`. That resumes you — the merge already landed, so nothing goes up for review.", prID, base, FileList(files))
}

// MsgResetFailed tells a worker its PR merged, but the hub hit a git error bringing its branch
// onto the new base afterward — unlike MsgReapplyConflict, this is not a known conflict shape, so
// it neither claims success nor points only at markers; `sindri resolve` may still find real ones.
func MsgResetFailed(prID, base string) string {
	return fmt.Sprintf("[hub] %s merged, but the hub hit an error bringing your branch onto %s afterward. Run `sindri resolve` — if it doesn't settle cleanly, say what it reports.", prID, base)
}

// MsgMilestoneRejected names the feature rather than the subtask in hand, since the PR covers the
// whole branch. A pointer, like its siblings below — DirContainerRejected re-serves the feedback.
func MsgMilestoneRejected(container, voice string) string {
	return fmt.Sprintf("[%s] The PR for feature %s was rejected. Run `sindri` for the feedback and "+
		"where to address it.", voice, container)
}

// MsgRejectedByUser tells a worker the user rejected its PR — a pointer, not the feedback itself,
// which stays on the PR (-> pr.Feedback) and is what DirRejected re-serves on every ask.
func MsgRejectedByUser(prID string) string {
	return fmt.Sprintf("[user] %s was rejected. Run `sindri` for the feedback and to carry on.", prID)
}

// MsgRejectedByAgent speaks in that agent's own voice: the role for a reviewer, the name for a
// coauthor. Same pointer shape as MsgRejectedByUser.
func MsgRejectedByAgent(voice, prID string) string {
	return fmt.Sprintf("[%s] %s was rejected. Run `sindri` for the feedback and to carry on.", voice, prID)
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
	return fmt.Sprintf("[hub] Review %s — %s %s(1) see what changed: %s. (2) check the gate: `sindri lint %s`. (3) decide: `sindri approve %s` or `sindri reject %s \"<findings>\"`.%s%s",
		prID, requirement, loc, seeChanges, prID, prID, prID, ReviewArchitecture(arch), ToolingBlock())
}
