# Action: Approve — delta

## ADDED Requirements

### Requirement: Approve a task with no PR

The system SHALL accept an approve action on a task that has no
associated PR, in which case approval closes the task with reason
"approved". Tasks that don't go through a PR (chores, discussion
items, anything that needs no code change) still need a way to signal
"this is done"; the natural meaning of "approve" on such a task is
"close as approved".

The close SHALL go through the same status transition the status
picker uses for a closed status, so the linked-spec lifecycle check
(see capability `spec-lifecycle`) fires for free — if the closed
task was the last open task carrying a `spec:<name>` label, the
spec is auto-archived or the user is prompted, exactly as if the
status had been changed to "closed" directly.

#### Scenario: Approve a task with no PR

- **GIVEN** task td-a has no associated PR
- **WHEN** the user presses `a` on td-a from the list view or the
  detail view
- **THEN** td-a is closed via the td adapter with reason "approved"
- **AND** the user sees a notification confirming the close

#### Scenario: Approve a task with no PR triggers spec lifecycle

- **GIVEN** task td-a carries `spec:X` and has no PR
- **AND** td-a is the only open task carrying `spec:X`
- **WHEN** the user approves td-a
- **THEN** td-a is closed
- **AND** the spec-lifecycle check fires (auto-archive or prompt
  depending on the spec's checklist completeness)
