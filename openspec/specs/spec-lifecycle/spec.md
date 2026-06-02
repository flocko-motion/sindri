# spec-lifecycle Specification

## Purpose
TBD - created by archiving change sync-task-spec-lifecycle. Update Purpose after archive.
## Requirements
### Requirement: Auto-archive when the last linked task closes

The system SHALL archive an openspec change automatically once every td
task carrying its `spec:<name>` label has closed AND the change's own
`tasks.md` checklist is complete (or has no checkboxes). Archive runs
`openspec archive <name> --yes` so the change moves under
`openspec/changes/archive/` with no human step.

#### Scenario: Last task closes and checklist is fully ticked

- **GIVEN** an active spec X whose checklist is N/N
- **AND** the only open task carrying `spec:X` is td-a
- **WHEN** td-a closes (via merge, status pick, or `td close`)
- **THEN** `openspec archive X --yes` runs
- **AND** the user is notified "Spec archived: X"

#### Scenario: Last task closes and there is no checklist

- **GIVEN** an active spec X whose checklist total is 0/0
- **WHEN** the last linked task closes
- **THEN** the spec archives without a prompt

#### Scenario: Last task closes but checklist is incomplete

- **GIVEN** an active spec X whose checklist is 2/5
- **WHEN** the last linked task closes
- **THEN** the system SHALL surface a confirm prompt — "Last task for
  spec X closed but checklist is 2/5. Archive anyway? (y/n)"
- **AND** `y` archives; `n` (or any other key) leaves the spec untouched

#### Scenario: Other linked tasks remain open

- **GIVEN** spec X has two open linked tasks td-a and td-b
- **WHEN** td-a closes
- **THEN** the spec SHALL NOT be archived
- **AND** no prompt SHALL appear

#### Scenario: Closed task had no spec link

- **WHEN** a task with no `spec:<name>` label closes
- **THEN** no spec-side action is taken

### Requirement: Abandoning a spec closes its linked open tasks

The system SHALL never leave a spec half-abandoned. When the user
abandons a spec, every open task carrying `spec:<name>` SHALL be closed
with reason "spec abandoned" before the change folder is removed. If
closing a linked task fails, the change folder is NOT removed and the
error is surfaced.

#### Scenario: Abandon an active spec with linked open tasks

- **GIVEN** active spec X with two open linked tasks td-a, td-b
- **WHEN** the user confirms abandon
- **THEN** td-a and td-b are closed with reason "spec abandoned"
- **AND** `openspec/changes/X/` is removed from disk
- **AND** the user is notified "Spec abandoned: X (closed 2 linked task(s))"

#### Scenario: Abandon a spec with no linked open tasks

- **GIVEN** active spec X with no open linked tasks
- **WHEN** the user confirms abandon
- **THEN** the change folder is removed
- **AND** the user is notified "Spec abandoned: X"

#### Scenario: Refuse abandon when the spec is already archived

- **GIVEN** spec X exists only under `openspec/changes/archive/...`
- **WHEN** the user attempts to abandon X
- **THEN** the system SHALL refuse with a visible error — archived specs
  are final

### Requirement: Orphan drift SHALL be visible

The system SHALL render a task whose `spec:<name>` label points at a
spec that is not an active proposal (archived externally, deleted, or
never existed) with a `⚠ spec archived` warning in its status column,
using the same warning style as the orphan-in_progress marker. This
makes drift introduced outside the sindri TUI visible at a glance.

#### Scenario: Open task whose spec is archived

- **GIVEN** task td-a carries `spec:X`
- **AND** spec X is under `openspec/changes/archive/...` only
- **WHEN** the work list renders td-a
- **THEN** its status cell shows `⚠ spec archived` instead of the regular
  task status

#### Scenario: Closed task whose spec is archived

- **GIVEN** task td-a is closed and carries `spec:X`
- **AND** spec X is archived
- **WHEN** the work list renders td-a (with `f` cycled to show closed)
- **THEN** the orphan warning is suppressed (closed is the steady state)

