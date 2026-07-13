// package: hub/workflow / prompts
// type:    logic (every agent-facing string in one place)
// job:     the agent's whole world is text the hub feeds it — the system prompt,
//
//	the no-arg `sindri` directives, the [hub]/[user]/[reviewer] injected
//	messages, and the replies to its verbs. Centralised here so the
//	agent's voice is tuned in one file, not scattered across the workflow.
//
// limits:  pure strings/builders; the logic that decides WHICH to use lives in
//
//	workflow_task.go / workflow_pr.go / hub.go.
package workflow

import (
	"fmt"
	"strings"
)

// DefaultReviewPrompt seeds the project's central review-prompt.txt the first time.
const DefaultReviewPrompt = "Review this PR for correctness, clarity, and fit to the task. Flag bugs, missing tests, and anything that should change."

// ReviewArchitecture is appended to every review instruction so a reviewer always
// (re-)reads the repo's architecture guide before ruling. The hub seeds an
// ARCHITECTURE.md into every repo it serves (see ensureArchitectureDoc), so there
// is always one to read.
// ReviewArchitecture builds the reviewer's "read the architecture doc" clause for the
// project's configured doc path (arch is repo-relative; /workspace is the mounted root).
func ReviewArchitecture(arch string) string {
	return fmt.Sprintf(" Read /workspace/%s now (even if you read it before) and confirm the changes follow it.", arch)
}

// ArchitecturePlaceholder seeds a repo's ARCHITECTURE.md when it has none, so the
// repo gains a home for the rules reviewers enforce. Deliberately minimal — a
// prompt to fill in, plus the one rule that already holds: conform to the brokkr
// linters.
const ArchitecturePlaceholder = "# Architecture\n" + `
<!-- Seeded by sindri. Describe how this project is meant to be built so reviewers
     can hold every change to it, then commit this file. Replace this comment. -->

## Baseline

All code must conform to the built-in ` + "`brokkr`" + ` linters (` + "`brokkr lint`" + `).
That floor is enforced automatically.

## Project rules

<!-- Add the rules that matter: layering and boundaries, what belongs where,
     naming, dependencies, patterns to follow or avoid. -->
_(none documented yet)_
`

// ArchitectureBrief injects the project's architecture INTO every agent's brief —
// the full doc content, so the agent always has it in context rather than a path it
// might never open — with a pointer to re-read the canonical copy. EVERY role needs
// this, not just the reviewer, or it can't produce work that fits. content is the
// doc's text; arch is its repo-relative path (/workspace is the mounted root). Empty
// when there's no architecture content to inject.
func ArchitectureBrief(content, arch string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\n\n# Project architecture (binding)\n\nThis is how the project is built and how your work must fit it — treat it as binding. To read it again at any time, refer to /workspace/%s.\n\n%s", arch, content)
}

// BrokkrBrief points every agent at the brokkr tool — always mounted into the pod.
// It's the recommended linter and a structured, grep-beating way to get an overview
// of the code, and it's built for single-command (no compound shell) usage — all of
// which the agent has to be told, since none of it is obvious from the binary alone.
func BrokkrBrief() string {
	return "\n\nThe `brokkr` tool is on your PATH — a toolbelt built for you (Claude Code): " +
		"every feature is a SINGLE self-contained command, so run it WITHOUT compound " +
		"shell — no pipes, no `&&`/`;`, no `2>&1 | tail`. Anything you'd reach a pipe for, " +
		"it already has a flag for (e.g. `--tail N` prints the last N lines AND the exit " +
		"status in one shot). So learn each subcommand from its own `--help` first " +
		"(`brokkr --help`, then `brokkr <cmd> --help`) — the help lists the flags that make " +
		"compound commands unnecessary. Prefer brokkr for two things: linting (`brokkr " +
		"lint`, the same gate `sindri lint` runs), and getting an overview of the codebase " +
		"— it maps structure and finds definitions/uses far better than grepping, so reach " +
		"for it before reading files blind."
}

// SystemPrompt is the agent's durable identity + how-to-work brief. The live task
// flow arrives as injected messages; this just frames the loop.
func SystemPrompt(name, role, archContent, archPath string) string {
	if role == "coauthor" {
		// A coauthor is NOT on the run-`sindri`-in-a-loop rails the other roles ride.
		// It shares the user's checkout and is driven directly, like an ordinary
		// pair-programming session — so its brief is deliberately different.
		return fmt.Sprintf(`You are %q, a Sindri coauthor running in a container that shares the
user's repository checkout at /workspace.

You work DIRECTLY with the user, like a normal pair-programming session. The user
types instructions into this terminal (you'll see lines prefixed [user]); act on
them. You have full read/write access to the repo at /workspace — edit files, run
the build and tests, and use git yourself (commit, branch, diff) as you normally
would. /workspace is the user's actual working copy: changes you make are changes
they see immediately, so move with the care you would in a shared tree. There is
no task queue and no review gate — you and the user steer the work together.

The `+"`sindri`"+` command offers a few optional helpers: `+"`sindri lint`"+` runs
the project's quality gate, `+"`sindri status`"+` shows who you are, and
`+"`sindri log \"<note>\"`"+` records a note in your activity log. You don't need
it to get work — the user gives you that here.

When the user goes quiet, stop and wait for their next instruction rather than
inventing work. Never poll or guess.`, name) + ArchitectureBrief(archContent, archPath) + BrokkrBrief()
	}

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
poll, never guess, never invent commands.`, name, role) + ArchitectureBrief(archContent, archPath) + BrokkrBrief()

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
- ` + "`sindri rebase`" + ` aligns your branch with the current reference branch
  any time — harmless, and worth doing if it's been a while. If it surfaces
  conflicts, fix the marked files in /workspace and run ` + "`sindri rebase`" + `
  again until it reports you're aligned.
- When done, ` + "`sindri submit \"<one-line summary>\"`" + `. Then wait: the
  reviewer's verdict will be typed here. Run ` + "`sindri`" + ` again for your
  next task.`
	}
}

// --- directives: the no-arg `sindri` answer (what to do next) ---

// FileList renders a blocking/conflicting-files list for an agent message: a plain
// join up to five, then a "+N more" tail so a huge conflict set stays readable.
func FileList(files []string) string {
	switch {
	case len(files) == 0:
		return "the conflicting files"
	case len(files) <= 5:
		return strings.Join(files, ", ")
	default:
		return strings.Join(files[:4], ", ") + fmt.Sprintf(", and %d more", len(files)-4)
	}
}

// DirWorking is a worker's directive while it holds a leaf task: implement it, then
// submit.
func DirWorking(task string) string {
	return fmt.Sprintf("Work on task %s. When your change is committed, run `sindri submit \"<summary>\"`.", task)
}

// DirRejected hands a worker its reviewer's feedback verbatim — pushed every time it
// asks the hub what to do, so a rejected PR's comments reach it whether or not it saw
// the moment-of-rejection message, and it never has to go dig them out of `sindri show`.
func DirRejected(task, feedback string) string {
	return fmt.Sprintf("Your PR for task %s was REJECTED — address this reviewer feedback, then run `sindri submit \"<summary>\"`:\n\n%s", task, feedback)
}

const DirSubmitted = "Your pull request is under review. Wait — the hub will tell you the verdict. While you wait, you may run `sindri resolve` any time to check your branch still merges onto its base (and resolve it if the base has moved) — it does no harm and keeps the PR healthy."

// DirPlanner is the idle planner's directive: orient, then wait for the user. A
// planner is never auto-assigned work.
const DirPlanner = "You're planning new features together with the user. Get oriented first: read README.md, read the backlog with `sindri task list` (and `sindri task <id>` for detail), and read the specs under /workspace/openspec. Then wait — the user will tell you what to plan. When you do: propose tasks with `sindri create-task \"<title>\"` (each needs the user's approval), draft specs in /workspace/openspec, and when a draft is ready — or whenever you want it reviewed — open a PR with `sindri openspec submit \"<summary>\"` (that PR is the review; don't ask the user to read your files instead). Only message the user for a concrete question you can't resolve yourself."

// DirCoauthor is the coauthor's no-arg `sindri` answer. It never blocks and never
// hands out managed work — the user drives a coauthor directly — so it just
// reorients: this is freestyle collaboration in the shared checkout.
const DirCoauthor = "You're a coauthor working directly with the user in the shared checkout at /workspace — there's no task queue here. Do what the user asks in this terminal; edit files, run the build/tests, and use git yourself. `sindri lint` runs the quality gate, `sindri log \"<note>\"` records a note. When the user goes quiet, wait for their next instruction."

// DirReview is a reviewer's directive: the PR branch is checked out fresh in its
// /workspace — read it, lint, then approve or reject.
func DirReview(prID, task, arch string) string {
	return fmt.Sprintf("Review %s (task %s): the PR branch is checked out fresh in /workspace — review it (or `sindri show %s`), run `sindri lint %s`, then `sindri approve %s` or `sindri reject %s \"<reason>\"`.%s",
		prID, task, prID, prID, prID, prID, ReviewArchitecture(arch))
}

// DirClaimed announces a freshly-claimed leaf task: the branch is ready in the
// worker's /workspace — work it, follow the architecture, then submit.
func DirClaimed(id, title, branch, arch string) string {
	return fmt.Sprintf("Claimed %s: %s\nBranch %s is ready in your /workspace. Work on it — follow the project architecture (in your brief; re-read it at /workspace/%s) — then run `sindri submit \"<summary>\"`.", id, title, branch, arch)
}

const DirNoTasks = "No open tasks. Wait — the hub will tell you when there is work."

// --- collaborative (container) workflow ---

// DirContainerClaimed starts an agent on a feature: it works the container's
// subtasks one at a time on a single standing branch, checkpointing (not
// submitting) between them. The whole feature lands as one PR when the user opens
// a milestone.
func DirContainerClaimed(container, ctitle, child, childTitle string) string {
	return fmt.Sprintf("You're working feature %s: %s — on a single branch in /workspace. "+
		"Current subtask %s: %s. Implement it, then run `sindri checkpoint \"<summary>\"` "+
		"to commit it and move to the next subtask. Do NOT submit per subtask — the whole feature "+
		"is merged as one PR when you and the user reach a milestone.", container, ctitle, child, childTitle)
}

// DirContainerWait is the directive once every open subtask of a feature is
// checkpointed: wait for the user to open a milestone PR or add subtasks.
func DirContainerWait(container string) string {
	return fmt.Sprintf("All open subtasks of feature %s are checkpointed. Wait — the user will open a "+
		"milestone PR to merge the work so far, or add more subtasks. Don't poll.", container)
}

// ReplyCheckpointed acknowledges a checkpoint and hands the worker its next subtask.
func ReplyCheckpointed(done, next, nextTitle string) string {
	return fmt.Sprintf("Checkpointed %s. Next subtask %s: %s — implement it, then `sindri checkpoint \"<summary>\"` again.", done, next, nextTitle)
}

// ReplyCheckpointedLast acknowledges the checkpoint that clears a feature's last open
// subtask: the worker now waits for a milestone PR or more subtasks.
func ReplyCheckpointedLast(done, container string) string {
	return fmt.Sprintf("Checkpointed %s — that was the last open subtask of %s. Wait: the user will open a milestone PR (or add more subtasks).", done, container)
}

const ReplyNothingToCheckpoint = "Nothing to checkpoint — you're not working a subtask. Run `sindri` for your current directive."

// --- injected messages ([hub]/[user]/[reviewer], typed into the agent's tmux) ---

const MsgKickoff = "[hub] You're live. Run `sindri` and do exactly what it tells you — it always returns your current job, whether you're new or resuming."

// MsgMerged tells a worker its PR merged and to fetch the next task.
func MsgMerged(prID string) string {
	return fmt.Sprintf("[hub] %s merged. Run `sindri` for your next task.", prID)
}

// MsgTaskCancelled tells a worker its task was closed/scrapped out from under it —
// stop, and don't clean up (the hub already reset the worktree), just get new work.
func MsgTaskCancelled(id string) string {
	return fmt.Sprintf("[hub] Task %s was cancelled — stop working on it. Don't clean up your workspace; the sindri hub will reset it for you when you pick up your next task. Just run `sindri`.", id)
}

// MsgRebased tells a worker the hub rebased its branch onto a moved base.
func MsgRebased(base string) string {
	return fmt.Sprintf("[hub] %s moved — your branch was rebased onto it, so you're up to date.", base)
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

// MsgRejectedByUser tells a worker the user rejected its PR, with the feedback.
func MsgRejectedByUser(prID, feedback string) string {
	return fmt.Sprintf("[user] %s was rejected: %s — address the feedback on your branch and run `sindri submit` again.", prID, feedback)
}

// MsgRejectedByReviewer tells a worker its reviewer rejected the PR, with the feedback.
func MsgRejectedByReviewer(prID, feedback string) string {
	return fmt.Sprintf("[reviewer] %s rejected: %s — please address the feedback and submit again.", prID, feedback)
}

// MsgReview is the single review instruction — the hub has already checked the PR
// branch out fresh into the reviewer's /workspace (the reviewer only reads), so it
// points there. If that checkout failed it says so loudly and falls back to the diff
// over the socket, so the reviewer never mistakes a stale tree for the PR.
func MsgReview(prID, requirement, branch, base, arch string, checkedOut bool) string {
	seeChanges := fmt.Sprintf("`sindri show %s`", prID)
	loc := ""
	if checkedOut {
		seeChanges = fmt.Sprintf("`git diff %s` in /workspace (or `sindri show %s`)", base, prID)
		loc = fmt.Sprintf("PR branch %s is checked out FRESH in /workspace, based on %s. ", branch, base)
	} else {
		// Loud: the checkout failed, so /workspace is NOT this PR. Say so plainly
		// rather than letting the reviewer assume /workspace holds the change.
		loc = fmt.Sprintf("⚠ %s could NOT be checked out into /workspace — review from the diff only; do NOT trust /workspace. ", branch)
	}
	return fmt.Sprintf("[hub] Review %s — %s %s(1) see what changed: %s. (2) check the gate: `sindri lint %s`. (3) decide: `sindri approve %s` or `sindri reject %s \"<findings>\"`.%s",
		prID, requirement, loc, seeChanges, prID, prID, prID, ReviewArchitecture(arch))
}

// --- instructive replies to worker verbs ---

// ReplyRegistered acknowledges a submitted PR and tells the worker to wait for review.
func ReplyRegistered(prID string) string {
	return fmt.Sprintf("%s registered. You'll be informed when it's reviewed. Please wait — this may take a while.", prID)
}

// ReplyRebaseConflicts answers `rebase` when the rebase hit conflicts to edit.
func ReplyRebaseConflicts(base string, files []string) string {
	return fmt.Sprintf("Rebasing onto %s hit conflicts in %s. They're in your /workspace with <<<<<<< markers — edit each file to the intended result (remove the markers), then run `sindri rebase` again to continue. Repeat until it reports you're aligned.", base, FileList(files))
}

// ReplyRebased answers `rebase` once the branch is cleanly current with base.
func ReplyRebased(base string) string {
	return fmt.Sprintf("Your branch is rebased onto %s — you're aligned with the current reference state. Carry on.", base)
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

const ReplyNothingToSubmit = "Nothing to submit — run `sindri` to pick up a task first."

// ReplyTaskProposed acknowledges a planner's proposed task, pending user approval.
func ReplyTaskProposed(id, title string) string {
	return fmt.Sprintf("Proposed %s: %s — awaiting the user's approval before any worker can pick it up.", id, title)
}

// ReplyLintFail answers a failed `sindri lint`, echoing the violations to fix.
func ReplyLintFail(out string) string {
	return fmt.Sprintf("Lint failed — fix the violations and submit again:\n%s", out)
}

// ReplySpecInvalid answers `openspec submit` when the change fails openspec's own
// validation (the planner's gate — the code linter doesn't apply to spec work).
func ReplySpecInvalid(out string) string {
	return fmt.Sprintf("openspec validation failed — fix the specs and submit again:\n%s", out)
}
