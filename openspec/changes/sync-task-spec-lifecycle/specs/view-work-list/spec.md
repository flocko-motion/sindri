# Work List View — delta

## MODIFIED Requirements

### Requirement: Approve and reject from the list view

`a` SHALL approve the PR of the task at the cursor; `x` SHALL reject the
task at the cursor — OR, when the cursor is on a spec-only row, `x` SHALL
open the abandon-spec confirm dialog instead (the same key, the row kind
picks the verb). From any other row, `x` is the existing reject flow.
Every failure mode SHALL surface a visible notification.

#### Scenario: Approve a task row that has a PR

- **GIVEN** the cursor is on a task row whose task has at least one PR
- **WHEN** the user presses `a`
- **THEN** the PR is approved

#### Scenario: Approve a row with no PR

- **GIVEN** the cursor is on a task row whose task has no PR
- **WHEN** the user presses `a`
- **THEN** the user sees "Approve: this task has no PR yet"

#### Scenario: Reject a task row

- **GIVEN** the cursor is on a task row
- **WHEN** the user presses `x`
- **THEN** the reject-reason input opens

#### Scenario: x on a spec-only row opens abandon

- **GIVEN** the cursor is on a spec-only row for spec X
- **WHEN** the user presses `x`
- **THEN** the bottom bar shows the abandon-spec confirm — "Abandon spec
  X? Deletes the change folder and closes its linked open tasks. (y/n)"
- **AND** `y` runs the abandon (closes linked open tasks, deletes the
  change folder); `n` (or any other key) cancels with no side effect

## ADDED Requirements

### Requirement: Drift warning for tasks whose spec is gone

The work list SHALL render an `⚠ spec archived` warning (red, bold) in
the status column of any open task whose `spec:<name>` label points at a
spec that is no longer an active proposal — same warning style as the
orphan-in_progress marker. The warning SHALL be suppressed once the task
itself is closed, since closed is the steady state.

#### Scenario: Open task with archived spec

- **GIVEN** task td-a is open and carries `spec:X`
- **AND** spec X is archived
- **WHEN** the work list renders td-a
- **THEN** its status column reads `⚠ spec archived`

#### Scenario: Closed task with archived spec

- **GIVEN** task td-a is closed and carries `spec:X`
- **AND** spec X is archived
- **WHEN** the work list renders td-a
- **THEN** the warning is suppressed and the status column shows the
  task's regular closed status
