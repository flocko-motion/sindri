// package: hub/workflow / prompts_feature
// type:    logic (the feature loop's agent-facing strings)
// job:     what a worker holding a FEATURE is told — claiming it, moving between its
// subtasks, why one is not finished yet, and what its unit gaining work means.
// limits:  pure strings; which one to use is the workflow's (-> feature.go), and the
// rest of the agent's voice stays in prompts.go.
package workflow

import "fmt"

// DirContainerClaimed starts an agent on a feature: subtasks one at a time on a single standing
// branch, checkpointing between them, and the branch goes up as one PR when they are all done.
func DirContainerClaimed(container, ctitle, child, childTitle string) string {
	return fmt.Sprintf("You're working feature %s: %s — on a single branch in /workspace. "+
		"Current subtask %s: %s. Implement it, then run `sindri checkpoint \"<summary>\"` "+
		"to record it and move to the next subtask. One PR covers the whole feature, so submit "+
		"once every subtask is checkpointed, never per subtask.%s", container, ctitle, child, childTitle, runPointer)
}

// DirContainerWorking is the working directive inside a feature. Claiming used to be the only place
// the feature loop named its verb; every later `sindri` fell through to DirWorking and asked for a
// submit that was held back mid-feature.
func DirContainerWorking(container, task string, aim, ceiling float64) string {
	return fmt.Sprintf("Subtask %s of feature %s. Implement it, then run `sindri checkpoint \"<summary>\"` "+
		"— that is what ends a subtask and hands you the next one; until you run it, %s stays yours and "+
		"you'll be given it again. Checkpointing records work on the feature branch and nothing more: "+
		"nothing of yours reaches the reference branch until a PR merges. The feature itself ends in ONE "+
		"pull request covering the whole branch — `sindri submit \"<summary>\"` once every subtask is "+
		"checkpointed, never per subtask. If what's on the branch is already useful to others, "+
		"`sindri contribute \"<summary>\"` puts it up for the user to merge without ending the feature.%s%s",
		task, container, task, CommentBudgetNote(aim, ceiling), ToolingBlock())
}

// DirContainerRejected is the verdict on a feature's PR: the worker fixes the branch it is already on
// and submits it again, the same loop a rejected leaf task follows.
func DirContainerRejected(container, task, feedback string, aim, ceiling float64) string {
	return fmt.Sprintf("The PR for feature %s was REJECTED — address this feedback on the branch you're "+
		"already on (subtask %s is yours again; `sindri checkpoint \"<summary>\"` records a fix that "+
		"completes it), then `sindri submit \"<summary>\"` to put the feature up again:\n\n%s%s%s",
		container, task, feedback, CommentBudgetNote(aim, ceiling), ToolingBlock())
}

// DirContainerDone is the directive once every subtask of a feature is checkpointed: the branch is
// complete, so the worker puts it up itself.
func DirContainerDone(container string) string {
	return fmt.Sprintf("Every subtask of feature %s is checkpointed, so the feature is finished. Put "+
		"the whole branch up with `sindri submit \"<summary>\"` — one PR for the feature, summarising "+
		"what it does rather than listing the subtasks.", container)
}

// plural picks a verb form for a count, so a refusal reads as English either way.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// ReplySubtasksRemain refuses a feature submitted early, naming what is left. A feature is one PR, so
// submitting halfway would put an incomplete branch under review.
func ReplySubtasksRemain(container, next string, open int) string {
	return fmt.Sprintf("Feature %s still has %d open subtask(s) and goes up as ONE PR. You're on %s — "+
		"`sindri checkpoint \"<summary>\"` records it and hands you the next; submit when they're done.",
		container, open, next)
}

// ReplyTaskGrew answers a submit whose task gained work after it was handed out. Not a refusal to
// leave the agent with: the same work is now a feature on the same branch, so it says what it has
// instead of a PR, and the PR it does put up later covers the whole of it.
func ReplyTaskGrew(id string, children []string) string {
	return fmt.Sprintf("%s gained work after you picked it up — %s now %s under it, so what you hold "+
		"is a FEATURE rather than one task, and its PR covers the whole of it. Nothing you have done "+
		"is lost: you stay on the same branch. Run `sindri` for the subtask, `sindri checkpoint "+
		"\"<summary>\"` to end each one, and submit when none are left.",
		id, FileList(children), plural(len(children), "sits", "sit"))
}

// ReplyCheckpointedParentOpen records a subtask that CANNOT close: it gained children of its own, so
// its work is its children's now. Said plainly, because "checkpointed" reads as finished and this
// one is not — it closes on its own once the work under it is done.
func ReplyCheckpointedParentOpen(done string, children []string, next, nextTitle string) string {
	return fmt.Sprintf("Recorded your work on %s. It stays OPEN: %s %s under it, and a task is done "+
		"exactly when its children are, so %s closes on its own once they do. Next subtask %s: %s — "+
		"it is yours and starts now.",
		done, FileList(children), plural(len(children), "is open", "are open"), done, next, nextTitle)
}

// ReplyFeatureGated refuses to call a feature finished while work under it awaits the user: that
// work is undone, merely absent from the queries that hand work out. Waiting is the whole answer —
// also used standalone as the directive `sindri` itself returns while this holds (-> claimNextSubtask).
func ReplyFeatureGated(container string, gated []string) string {
	return fmt.Sprintf("Feature %s isn't finished: %s under it %s awaiting the user's approval, so "+
		"that is work still to do rather than work you have done. You'll be pushed a wake once the "+
		"user rules — asking again meanwhile just reads the same wait.",
		container, FileList(gated), plural(len(gated), "is", "are"))
}

// ReplyCheckpointed acknowledges a checkpoint and hands over the next subtask. Alone among the
// hand-offs it comes back from a command the worker ran itself, and read as a report it left agents
// waiting for a go-ahead the workflow never sends — hence "starts now".
func ReplyCheckpointed(done, next, nextTitle string) string {
	return fmt.Sprintf("Checkpointed %s. Next subtask %s: %s — it is assigned to you and starts now. "+
		"Implement it, then `sindri checkpoint \"<summary>\"` again. Don't wait for the user to confirm "+
		"this one; if something blocks you, say what you need.", done, next, nextTitle)
}

// ReplyCheckpointedLast acknowledges the checkpoint that clears a feature's last open subtask: the
// feature is built, so the worker puts the branch up rather than waiting to be let through.
func ReplyCheckpointedLast(done, container string) string {
	return fmt.Sprintf("Checkpointed %s — the last open subtask of %s, so the feature is done. Put the "+
		"whole branch up now with `sindri submit \"<summary>\"`.", done, container)
}

const ReplyNothingToCheckpoint = "Nothing to checkpoint — you're not working a subtask. Run `sindri` for your current directive."

// ReplyCheckpointedClearing answers a checkpoint made while a clear was armed: the feature stays
// held, the next subtask waits for the empty session.
func ReplyCheckpointedClearing(done, container string) string {
	return fmt.Sprintf("Checkpointed %s. The user has armed a context clear, so %s's next subtask "+
		"waits for it: your session is about to be cleared, and you'll be told to carry on after. "+
		"Wait — don't ask for the next one.", done, container)
}
