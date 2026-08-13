# A planner may edit any task, and every edit returns it to review

## Why

`edit-task` refuses the moment the user approves anything: "only a task still awaiting the user's
approval can be edited". Working the task list is what a planner is for, and that refusal stops it
at the first approval. In one session it blocked bundling five presentation tasks under an epic,
grouping two colliding keymap tasks, and correcting a task body whose premise had turned out to be
false — each time handing the user commands to run by hand for work the planner had already
reasoned about.

The refusal reads approval as permission. It is not. Approval is the user's record that they have
READ a task, and priority is their control over what starts now; neither guards the hub against its
own agents. A planner needs no permission to revise a task. What its edit does is make the user's
record untrue — so the record is cleared and the task goes back for a fresh one.

Reading the gates as guards is what produced the wrong designs on this task twice over: forbidding
the edit outright, and then gating it behind a queue of pending changes. The second would need a
pending-change record, a state for it, an approval surface in both front-ends, and answers for
stacked edits and for a task moving underneath one — a second approval workflow, harder to model
than task creation because an edit is a delta against a moving target, all to guard changes whose
worst outcome is that something reads wrongly until it is fixed.

## What changes

- `edit-task` accepts any task a planner can see, whatever its approval state. The task exists is
  the only precondition left; the approval read that used to refuse an absent id by accident is
  replaced by a check of its own.
- Every edit that lands returns the task to awaiting-review. ONE rule, no split by field: deciding
  which fields are "substantive" means a classification nothing tests, which drifts the first time
  a field is added.
- That also pauses release, correctly — both claim pools require approval, so a task whose
  definition just changed is not handed out before the user has seen the change. It does not reach
  a worker already holding it: the claim gate is about handing work OUT, and a holder finishes and
  submits exactly as before.
- Inside a feature, "is it finished" stops being asked of `OpenSubtasks`. That query answers what is
  workable NOW, so a gated subtask does not come back from it held back — it is absent, which reads
  as finished. The worker was told the feature was complete, submitted the branch, and the feature
  closed over a subtask nobody had worked; the reconciler is only a partial net, since it reads
  direct children while the assignment query reaches any depth. Completion now asks `gatedUnder`:
  open work still AWAITING a verdict, at any depth. With nothing workable and nothing finished,
  `sindri` WAITS — the approval's own notify wakes it, so no polling and no new nudge. The
  fragility in `OpenSubtasks` predates this change and a user's rejection reaches it today; what
  this change does is make un-approval the routine consequence of the planner's main verb.
- Pending blocks completion; REJECTED does not, and the claim rule is the wrong one here. Nothing
  clears a rejection — the wait ends when the user rules, and on a rejected task they have — so
  blocking on one parks the holder indefinitely, with no verb able to clear it and no message
  saying why. That is a worse failure than the wrong completion, not a safer one. Assignment asks a
  different question ("is the plan settled?") and is sd-d38859's; the two must not share a helper.
- The edit is recorded on the task, carrying the value each changed field held before it. The old
  value survives nowhere else once the write lands, and "something was edited" is not a record. The
  verdict being cleared goes in too: a rejection's reason is held in the approval row alone, and
  this is the write that erases it — revising a rejected task is the ordinary flow, and it was
  impossible under the old rule, so the hole opens with the change that allows it.
- Every agent whose UNIT of work the edit touches is told — the holder of the edited task, and the
  holder of anything above it at any depth. A worker holds a feature, not a row, so matching the
  edited row told nobody in the case that bites: an edit to a subtask nobody holds, inside a feature
  someone does. The note names which task changed and how, says whether it is the agent's own work
  or sits inside it, points at the record, and says the agent's own work is still its to finish — a
  task showing "pending" again otherwise reads as one taken away.
- What actually moved is read off the stored rows either side of the write, never echoed back from
  the request. A task whose content its own source owns absorbs no edit, and reporting one would
  claim a change that never happened and spend the user's verdict for it.
- `edit-task` no longer accepts `--priority`, naming `prioritise-task` instead. One field cannot
  have two verbs with opposite consequences: an edit returns the task for a verdict, a re-ordering
  leaves the verdict standing. Nothing is lost — every rating `edit-task` could reach under the old
  pending-only rule, `prioritise-task` could reach too.
- The user's own edit is untouched: the user editing a task is the user reading it.

## Impact

- Specs: `hub` gains the requirement for editing, and "A planner may order work, never authorise
  it" is modified — its absolute "no planner action changes whether a task is claimable" was
  written about ratings and now has to say which direction it forbids. Returning a task to the user
  hands the decision back rather than taking it, which is the opposite of releasing work unasked.
- Code: `internal/hub/workflow/planner.go` (the verb, the record, the holder), `injected.go`
  (the note), `feature.go` (`gatedUnder`, `containerNext`, the checkpoint reply), `pr.go` (the
  submit gate), `task.go` (the held-feature directive now waits), `prompts.go`,
  `plannerpriority.go` and `reopen.go` (comments that described the old rule).
- `workflow.Deps` gains `AddTaskComment`, the write half of the `TaskComments` it already had:
  the record and the note are two halves of telling the same person, and splitting them across
  packages to avoid one seam method would be the worse trade.
