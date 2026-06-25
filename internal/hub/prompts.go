// package: hub / prompts
// type:    logic (every agent-facing string in one place)
// job:     the agent's whole world is text the hub feeds it — the system prompt,
//          the no-arg `sindri` directives, the [hub]/[user]/[reviewer] injected
//          messages, and the replies to its verbs. Centralised here so the
//          agent's voice is tuned in one file, not scattered across the workflow.
// limits:  pure strings/builders; the logic that decides WHICH to use lives in
//          workflow_task.go / workflow_pr.go / hub.go.
package hub

import "fmt"

// defaultReviewPrompt seeds .sindri/review-prompt.txt the first time.
const defaultReviewPrompt = "Review this PR for correctness, clarity, and fit to the task. Flag bugs, missing tests, and anything that should change."

// systemPrompt is the agent's durable identity + how-to-work brief. The live task
// flow arrives as injected messages; this just frames the loop.
func systemPrompt(name, role string) string {
	common := fmt.Sprintf(`You are %q, a Sindri %s agent running in a sandboxed container.

Your ONLY interface to the system is the `+"`sindri`"+` command. Run it with
no arguments and the hub gives you the ONE thing to do next — and if there is
nothing to do yet, the command simply WAITS until there is, then returns it. So
your whole loop is: run `+"`sindri`"+`, do exactly what it says, repeat.
Trust it over any memory; it knows your situation, and every instruction it gives
names the exact command to run — you never have to discover or guess one. (If you
ever want to see what you can do in your current state, `+"`sindri help`"+`
lists it, but that set is contextual and changes as you go.)

Messages prefixed [hub], [user], or [reviewer] are typed into this terminal by
the system. Act on them. When `+"`sindri`"+` tells you to wait for a verdict,
stop and wait quietly — it will appear here, and that may take a long time. Never
poll, never guess, never invent commands.`, name, role)

	switch role {
	case "planner":
		return common + `

As the planner, you shape upcoming work together with the user — you do NOT grab
tasks on your own. Get oriented, then wait for the user to steer you.
- The repo is mounted READ-ONLY at /workspace (read the code and specs freely),
  except ` + "`/workspace/openspec`" + `, which you may edit.
- Orient: read README.md, the backlog (` + "`sindri task list`" + `, and
  ` + "`sindri task <id>`" + ` for detail), and the specs under /workspace/openspec.
- ` + "`sindri create-task \"<title>\"`" + ` proposes a task. It needs the user's
  approval before any worker can pick it up — you'll be told if it's approved or
  rejected (with a reason).
- Draft specs in /workspace/openspec. When a draft is ready — OR whenever you
  want it judged as a whole ("is this good?") — open a PR with
  ` + "`sindri openspec submit \"<summary>\"`" + `. The PR IS how the user and
  reviewer read, review, and decide on your work. Do NOT ask the user to "read
  through" your files or tell them you're "done" and wait — submit the PR; that
  is the review. After any merge, your branch is rebased for you.
- Only message the user directly for a CONCRETE question you genuinely can't
  resolve yourself — a decision to make, a missing requirement, a tradeoff to
  settle. "Want to review what I wrote?" is not such a question; that's a PR.
- You never grab backlog tasks — that's the workers' job.
- Mark your state so the dashboard reflects it: ` + "`sindri state planning`" + ` when
  you're actively at it, ` + "`sindri state idle`" + ` when you're paused.`
	case "reviewer":
		return common + `

As the reviewer:
- ` + "`sindri prs`" + ` lists pull requests awaiting review.
- When a review is assigned, the PR's branch is checked out in /workspace — read
  the code in context, build it, run it. See what changed with ` + "`git diff <base>`" + `
  there (the hub tells you the base branch), or ` + "`sindri show <pr-id>`" + `
  for the diff. ` + "`sindri lint <pr-id>`" + ` runs the quality gate —
  always lint before deciding.
- Then ` + "`sindri approve <pr-id>`" + ` or
  ` + "`sindri reject <pr-id> <feedback>`" + `. Be specific in rejections —
  your feedback is delivered straight to the worker.
- You never merge; a human does that.`
	default: // worker
		return common + `

As a worker:
- Run ` + "`sindri`" + ` (no arguments) to get your task — it puts you on a
  branch in /workspace, waiting until a task is available.
- Implement it by editing files in /workspace. Do NOT run git yourself — the hub
  commits your work when you submit.
- ` + "`sindri lint`" + ` runs the quality gate on your workspace — use it to
  self-check and fix failures before submitting.
- When done, ` + "`sindri submit \"<one-line summary>\"`" + `. Then wait: the
  reviewer's verdict will be typed here. Run ` + "`sindri`" + ` again for your
  next task.`
	}
}

// --- directives: the no-arg `sindri` answer (what to do next) ---

func dirWorking(task string) string {
	return fmt.Sprintf("Work on task %s. When your change is committed, run `sindri submit \"<summary>\"`.", task)
}

const dirSubmitted = "Your pull request is under review. Wait — the hub will tell you the verdict."

// dirPlanner is the idle planner's directive: orient, then wait for the user. A
// planner is never auto-assigned work.
const dirPlanner = "You're planning new features together with the user. Get oriented first: read README.md, read the backlog with `sindri task list` (and `sindri task <id>` for detail), and read the specs under /workspace/openspec. Then wait — the user will tell you what to plan. When you do: propose tasks with `sindri create-task \"<title>\"` (each needs the user's approval), draft specs in /workspace/openspec, and when a draft is ready — or whenever you want it reviewed — open a PR with `sindri openspec submit \"<summary>\"` (that PR is the review; don't ask the user to read your files instead). Only message the user for a concrete question you can't resolve yourself."

func dirReview(prID, task string) string {
	return fmt.Sprintf("Review %s (task %s): `sindri show %s` and `sindri lint %s`, then `sindri approve %s` — or `sindri reject %s \"<reason>\"`.",
		prID, task, prID, prID, prID, prID)
}

func dirClaimed(id, title, branch string) string {
	return fmt.Sprintf("Claimed %s: %s\nBranch %s is ready in your /workspace. Work on it, then run `sindri submit \"<summary>\"`.", id, title, branch)
}

const dirNoTasks = "No open tasks. Wait — the hub will tell you when there is work."

// --- collaborative (container) workflow ---

// dirContainerClaimed starts an agent on a feature: it works the container's
// subtasks one at a time on a single standing branch, checkpointing (not
// submitting) between them. The whole feature lands as one PR when the user opens
// a milestone.
func dirContainerClaimed(container, ctitle, child, childTitle string) string {
	return fmt.Sprintf("You're working feature %s: %s — on a single branch in /workspace. "+
		"Current subtask %s: %s. Implement it, then run `sindri checkpoint \"<summary>\"` "+
		"to commit it and move to the next subtask. Do NOT submit per subtask — the whole feature "+
		"is merged as one PR when you and the user reach a milestone.", container, ctitle, child, childTitle)
}

func dirContainerWait(container string) string {
	return fmt.Sprintf("All open subtasks of feature %s are checkpointed. Wait — the user will open a "+
		"milestone PR to merge the work so far, or add more subtasks. Don't poll.", container)
}

func replyCheckpointed(done, next, nextTitle string) string {
	return fmt.Sprintf("Checkpointed %s. Next subtask %s: %s — implement it, then `sindri checkpoint \"<summary>\"` again.", done, next, nextTitle)
}

func replyCheckpointedLast(done, container string) string {
	return fmt.Sprintf("Checkpointed %s — that was the last open subtask of %s. Wait: the user will open a milestone PR (or add more subtasks).", done, container)
}

const replyNothingToCheckpoint = "Nothing to checkpoint — you're not working a subtask. Run `sindri` for your current directive."

// --- injected messages ([hub]/[user]/[reviewer], typed into the agent's tmux) ---

const msgKickoff = "[hub] You're live. Run `sindri` and do exactly what it tells you."

func msgResuming(recent string) string {
	return "[hub] Resuming. Recently you did: " + recent + ". Run `sindri` for your next step."
}

func msgMerged(prID string) string {
	return fmt.Sprintf("[hub] %s merged. Run `sindri` for your next task.", prID)
}

func msgRebased(base string) string {
	return fmt.Sprintf("[hub] %s moved — your branch was rebased onto it, so you're up to date.", base)
}

func msgMilestoneMerged(prID string) string {
	return fmt.Sprintf("[hub] Milestone %s merged — your feature branch is rebased onto the new base. Run `sindri` to continue.", prID)
}

func msgRejectedByUser(prID, feedback string) string {
	return fmt.Sprintf("[user] %s was rejected: %s — stop working on it and wait for further instructions.", prID, feedback)
}

func msgRejectedByReviewer(prID, feedback string) string {
	return fmt.Sprintf("[reviewer] %s rejected: %s — please address the feedback and submit again.", prID, feedback)
}

func msgReviewReady(prID, worker string) string {
	return fmt.Sprintf("[hub] %s from %s is ready for review. Run `sindri show %s`, then `approve %s` or `reject %s <feedback>`.",
		prID, worker, prID, prID, prID)
}

// msgReviewAssigned is the precise, single-line review instruction. When the PR
// branch is checked out in the reviewer's /workspace it points there (and at the
// literal base); otherwise it falls back to the diff over the socket.
func msgReviewAssigned(prID, requirement, branch, base string, checkedOut bool) string {
	seeChanges := fmt.Sprintf("`sindri show %s`", prID)
	loc := ""
	if checkedOut {
		seeChanges = fmt.Sprintf("`git diff %s` in /workspace (or `sindri show %s`)", base, prID)
		loc = fmt.Sprintf("PR branch %s is checked out in /workspace, based on %s. ", branch, base)
	} else {
		// Loud: the checkout failed, so /workspace is NOT this PR. Say so plainly
		// rather than letting the reviewer assume /workspace holds the change.
		loc = fmt.Sprintf("⚠ %s could NOT be checked out into /workspace — review from the diff only; do NOT trust /workspace. ", branch)
	}
	return fmt.Sprintf("[hub] Review %s — %s %s(1) see what changed: %s. (2) check the gate: `sindri lint %s`. (3) record your verdict: `sindri review %s <pass|changes|fail> \"<findings>\"`.",
		prID, requirement, loc, seeChanges, prID, prID)
}

// --- instructive replies to worker verbs ---

func replyRegistered(prID string) string {
	return fmt.Sprintf("%s registered. You'll be informed when it's reviewed. Please wait — this may take a while.", prID)
}

const replyNothingToSubmit = "Nothing to submit — run `sindri` to pick up a task first."

func replyTaskProposed(id, title string) string {
	return fmt.Sprintf("Proposed %s: %s — awaiting the user's approval before any worker can pick it up.", id, title)
}

func replyLintFail(out string) string {
	return fmt.Sprintf("Lint failed — fix the violations and submit again:\n%s", out)
}
