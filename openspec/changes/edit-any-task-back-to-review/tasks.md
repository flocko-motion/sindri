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

## 3. Tell the holder

- [x] 3.1 A worker holding the task (or the feature it sits in) is told what changed, sent back to
      the task for the full record, and told the work is still its own to finish.
- [x] 3.2 A holder that is down is not waited on — the record on the task is the durable half, and
      the planner is told which it was.

## 4. Pin it

- [x] 4.1 Every approval state is editable and every one lands on pending.
- [x] 4.2 An edited task leaves the claim pool; its holder still gets the directive to finish and
      submit it.
- [x] 4.3 The record carries the old value and the new; the holder's note names the task and the
      fields.
- [x] 4.4 An unknown id is refused, a rating is sent to `prioritise-task` writing nothing at all,
      and an edit to a task sindri does not own leaves the approval alone.
