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

On a successful merge, the associated task SHALL be closed. If the closed
task carries a `spec:<name>` label, the system SHALL ALSO consult the
spec lifecycle (see capability `spec-lifecycle`) and either auto-archive
the spec or surface a confirm prompt — depending on whether other open
tasks still link to the spec and whether the spec's checklist is
complete.

#### Scenario: Task closed on merge

- **WHEN** a PR for a task is merged
- **THEN** that task is closed

#### Scenario: Linked task merged, spec ready to archive

- **GIVEN** the merged task was the last open task carrying `spec:X`
- **AND** spec X's checklist is N/N
- **WHEN** the merge succeeds
- **THEN** the task closes AND `openspec archive X --yes` runs
- **AND** the user sees both notifications

#### Scenario: Linked task merged, spec checklist incomplete

- **GIVEN** the merged task was the last open task carrying `spec:X`
- **AND** spec X's checklist is 2/5
- **WHEN** the merge succeeds
- **THEN** the task closes AND the user is prompted whether to archive X
  anyway

### Requirement: Human-only

Merging SHALL require explicit confirmation that a human is acting; agents SHALL
NOT merge their own work.

#### Scenario: Confirmation required

- **WHEN** merge is invoked
- **THEN** it confirms a human is acting before merging

