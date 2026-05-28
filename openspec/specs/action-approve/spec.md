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
