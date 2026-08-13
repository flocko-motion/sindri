# Tasks

## 1. Let the edit through

- [x] 1.1 `CmdEditTask` drops the pending-only refusal and checks the task exists instead — the
      approval read was refusing an absent id by accident, and without it an edit to nothing
      reported success.
- [x] 1.2 Every edit that lands writes the approval back to pending. One rule, no field split.
- [x] 1.3 `--priority` is refused, naming `prioritise-task`: an edit returns the task for a
      verdict and a re-ordering must not, so one field cannot carry both.

## 2. Say what changed

- [x] 2.1 The change is computed from the stored rows either side of the write, so a field a
      task's own source owns reports honestly as unchanged.
- [x] 2.2 An edit that moved nothing spends no verdict and says which case it was — already
      reading that way, or content its own source keeps.
- [x] 2.3 The record goes on the task's thread, with the value each changed field held before it.
- [x] 2.4 The verdict being cleared goes into that record too — a rejection's reason lives in the
      approval row alone, and this write is what erases it. Revising a rejected task is the
      ordinary flow, and was impossible under the old rule, so the hole is new.

## 3. Never finish a feature over gated work

- [x] 3.1 `gatedUnder` answers the completion question at any depth, applying the same
      `authorisedForClaim` rule the claim queries do — `OpenSubtasks` answers "workable now", where
      gated work is absent rather than reported, and absent reads as finished.
- [x] 3.2 The checkpoint reply, the held-feature directive and the submit gate all ask it. With
      nothing workable and nothing finished the directive WAITS; the approval's notify wakes it.
- [x] 3.3 The submit gate is the backstop, so the narrow race where a feature is claimed as
      finished cannot end in a PR over undone work.

## 4. Tell the holder

- [x] 4.1 A worker holding the task (or the feature it sits in) is told what changed, sent back to
      the task for the full record, and told the work is still its own to finish.
- [x] 4.2 A holder that is down is not waited on — the record on the task is what reaches it when
      it comes back, and the planner is told which case it was.
- [x] 4.3 Told BEFORE the record is written, so the one path where the record does not exist is not
      also the path where the reader who most needs it hears nothing. That reply says the task
      carries no record and names who was told.

## 5. Pin it

- [x] 5.1 Every approval state is editable and every one lands on pending.
- [x] 5.2 An edited task leaves the claim pool; its holder still gets the directive to finish and
      submit it.
- [x] 5.3 The record carries the old value and the new, and the verdict it replaces; the holder's
      note names the task and the fields; a failed record still tells the holder.
- [x] 5.4 An unknown id is refused, a rating is sent to `prioritise-task` writing nothing at all,
      and an edit to a task sindri does not own leaves the approval alone.
- [x] 5.5 The feature case end to end — hold a feature, gate an unstarted sibling, and the
      checkpoint does not say "submit", the submit is refused with no PR, the directive waits, and
      the approval releases the subtask. Mutation-checked: disabling any of the three guards fails
      it, reproducing the reported defect exactly.
