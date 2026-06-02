## ADDED Requirements

### Requirement: Approve and reject from the list view

The work list SHALL let the user approve or reject a task without first
opening the detail view. `a` pressed in the list view, with the cursor on a
task row whose Issue has an open PR, SHALL approve that PR using the same
shared logic the detail view's `a` uses (the `action.Approve` path), and SHALL
update the row optimistically. `x` pressed in the list view, with the cursor
on a task row, SHALL enter the reject-reason input flow (same input flow the
detail view's `x` uses); on submission the task is rejected using
`action.Reject` for the cursor row's PR (or `action.RejectTask` if no PR
exists yet).

Every failure mode SHALL produce a visible notification — pressing `a` on a
non-task row, on a task with no PR, or on a PR that cannot be approved, SHALL
NOT silently do nothing. The detail view's existing `a` and `m` bindings,
which previously fell through silently when `m.detail.prIDs` was empty, SHALL
also surface a visible notification in that case.

#### Scenario: Approve from the list view

- **WHEN** the user presses `a` while the cursor is on a task row that has an
  associated open PR
- **THEN** that PR is approved through the shared action and the user sees a
  confirmation notification

#### Scenario: Reject from the list view

- **WHEN** the user presses `x` while the cursor is on a task row
- **THEN** the reject-reason input opens, and on submission the task is
  rejected with the typed reason via the shared action

#### Scenario: No PR yet

- **WHEN** the user presses `a` on a task that has no PR yet, in either the
  list or detail view
- **THEN** a notification "Approve: this task has no PR yet" is shown and the
  key never silently does nothing

#### Scenario: Non-task row

- **WHEN** the user presses `a` or `x` on a spec-only row, a PR sub-row, or
  the workers panel
- **THEN** a visible notification names the constraint ("Approve: pick a task
  row first" / "Reject: pick a task row first")
