# Action: Merge

## Purpose

Defines merging a pull request into its base branch, independent of any
interface. Merge is the human act that lands work and closes its task.

## Requirements

### Requirement: Merge into base

Merging SHALL merge the PR's branch into its base branch and mark the PR merged.
The PR's branch SHALL be deleted after a successful merge.

#### Scenario: Merge approved PR

- **WHEN** an approved PR is merged
- **THEN** its branch is merged into base and the PR becomes merged

### Requirement: Gated by reviews

Merge SHALL be refused while any required review gate on the task is unmet,
listing the missing gates.

#### Scenario: Unmet gate

- **WHEN** merge is attempted with an unmet review gate
- **THEN** it is refused and the missing gate is named

### Requirement: Closes the task

On a successful merge, the associated task SHALL be closed.

#### Scenario: Task closed on merge

- **WHEN** a PR for a task is merged
- **THEN** that task is closed

### Requirement: Human-only

Merging SHALL require explicit confirmation that a human is acting; agents SHALL
NOT merge their own work.

#### Scenario: Confirmation required

- **WHEN** merge is invoked
- **THEN** it confirms a human is acting before merging
