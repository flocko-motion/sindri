## Why

`td-e035de`: when a task is in_review the user wants to approve or reject it
without first opening the detail view. Also, two of the existing detail-view
bindings — `a` (approve) and `m` (merge) — silently do nothing when the task
has no PR yet (just `if len(prIDs) > 0 { … }` with no else), so the user sees
no feedback at all and assumes the key is broken.

## What Changes

- Add `a` (approve) and `x` (reject) bindings in the **list view**:
  - `a` on a task row whose Issue has an open PR → approve that PR (same
    `approvePR` path as detail). The task's prIDs are stashed in `m.detail`
    first so the existing action reads them.
  - `x` on a task row → enter the reject-reason input (same flow as detail's
    `x`). The task and its PR id are stashed in `m.detail` first.
  - Both `a` and `x` on a non-task row, or a task row without an open PR,
    surface a visible notification ("Approve: this task has no PR yet" /
    "Reject: pick a task row first") instead of doing nothing.
- Fix the detail-view silent fallthrough: `a` and `m` now also surface a
  visible notification when `m.detail.prIDs` is empty, so the user always sees
  feedback.

## Capabilities

### Modified Capabilities

- `view-work-list`: add "Approve and reject from the list view" requirement
  describing the `a` / `x` bindings and the cursor → PR resolution.

## Impact

- `internal/tui/actions.go` — add a `cursorTaskAndPR()` helper, and
  `enterRejectMode(taskID, prID)` so the same setup runs from both views.
- `internal/tui/tui.go` — handle `a` and `x` in the list-view key block (with
  visible notifications on every failure mode); patch detail-view `a` and `m`
  to notify when there's no PR.
- `internal/tui/replay_test.go` + `testdata/frames/` — new golden
  `list-approve-no-pr` capturing the "no PR yet" notification path so the
  visible-error behaviour is regression-protected.
- No CLI change.
