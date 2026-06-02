# Action: Merge — delta

## MODIFIED Requirements

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
