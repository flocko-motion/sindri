# Action: Approve

## Purpose

Defines approving a pull request, independent of any interface. Approval marks a
PR ready and satisfies its review gates. It is the reviewer's decision (the
reviewer agent via `sindri-review pr approve`, or a human on the host) and does
NOT require human confirmation — only merge does.
## Requirements
### Requirement: Approve a PR

Approving SHALL move an open PR to the approved state in the PR store. A merged
PR SHALL NOT be approvable.

#### Scenario: Approve open PR

- **WHEN** an open PR is approved
- **THEN** its status becomes approved

#### Scenario: Merged PR

- **WHEN** approve is invoked on a merged PR
- **THEN** it is refused

### Requirement: Satisfies review gates

Approving SHALL satisfy the task's review gates by adding the matching
`approved-review-*` label for each unmet `require-review-*` gate, so a later
merge is unblocked.

#### Scenario: Gate satisfied

- **WHEN** a PR whose task requires `require-review-code` is approved
- **THEN** the task gains `approved-review-code`

### Requirement: Not self-approval

Review SHALL be performed by an actor other than the one that built the work: an
agent SHALL NOT approve its own PR. The reviewer agent reviews the worker's PRs.

#### Scenario: Separate reviewer

- **WHEN** a worker's PR is approved
- **THEN** the approval comes from the reviewer agent or a human, not the worker

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

