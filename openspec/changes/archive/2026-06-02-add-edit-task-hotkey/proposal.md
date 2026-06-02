# Add `e` hotkey to edit a task

## Why

The TUI today can comment on a task (`c`), change its status (`s`), move it
in the hierarchy (`m`), approve/reject/merge — but it can't edit the task
itself. To fix a title typo, change the priority, change the type, or
toggle the review gate, the user has to drop to the `td` CLI. That's a
context switch for what should be a single keystroke.

## What Changes

New binding `e` available on both the list view (cursor on a task row)
and the detail view (kind == task). It opens the existing new-task modal
in **edit mode**: pre-populated with the task's current title, type,
priority, and review-gate setting; submit dispatches `td update` instead
of `td create`. Other labels carried by the task (`spec:<name>`,
`approved-review-*`, etc.) are preserved across the edit so toggling
the review checkbox can't silently drop them.

Editing skips the 15-character minimum-title rule that applies on
create — tasks created via the `td` CLI may have shorter titles and
forcing the user to lengthen them just to change the priority would
be obnoxious. The minimum still applies to new tasks.

The description field is intentionally optional on edit and labeled
"leave empty to keep current": td's update only changes description
when `-d` is passed, so leaving it blank preserves whatever description
the task already had.

## Impact

- New capability `action-edit-task` captures the edit semantics in one
  place (mirrors `action-create-task`).
- `view-work-list` and `view-item-detail` MODIFIED: both list `e` as a
  binding, both describe the pre-fill behaviour.
- Adapter: `td.UpdateOpts` + `td.Update(root, id, opts)` added to
  `internal/adapter/td`. Only non-empty/non-nil fields are sent so the
  caller can submit a partial change.
- TUI: `newEditTaskModel` builds the modal from an `issue.Task`;
  `createTaskModel` gains `editingID` and `origLabels`; submit branches
  on `editingID`. New `taskUpdatedMsg` mirrors `taskCreatedMsg`.
- Goldens: new `edit-task` capture; existing goldens untouched (no
  layout change to the list).
