# Action: Reject

## Purpose

Defines rejecting a pull request and returning its task for rework, independent
of any interface. Rejection sends work back with feedback.

## Requirements

### Requirement: Reject with a reason

Rejecting SHALL require a reason. The reason SHALL be added as a comment on the
task before the task is returned to open, so the worker sees the feedback.

#### Scenario: Reject with feedback

- **WHEN** a PR is rejected with a reason
- **THEN** the reason is commented on the task and the task returns to open

#### Scenario: No reason

- **WHEN** rejection is attempted with no reason
- **THEN** it is refused

### Requirement: PR marked rejected

The rejected PR SHALL be marked rejected in the store; its open/approved
sibling PRs for the same task SHALL also be closed out.

#### Scenario: PR state on reject

- **WHEN** a task is rejected
- **THEN** its open/approved PRs are marked rejected

### Requirement: Worker picks it up again

Because the task returns to open, the next `sindri-worker issue next` SHALL surface it
again with the rejection comment for the worker to act on.

#### Scenario: Rework

- **WHEN** a worker runs `sindri-worker issue next` after a rejection
- **THEN** the rejected task can be picked up again with its feedback visible
