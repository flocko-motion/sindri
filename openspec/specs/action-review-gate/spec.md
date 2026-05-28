# Action: Review Gate

## Purpose

Defines marking a review gate as satisfied on a task, independent of any
interface. A reviewer marks each required review (e.g. code) approved; merge is
gated on all required reviews being approved.

## Requirements

### Requirement: Mark a gate approved

Approving review gate `<x>` SHALL add `approved-review-<x>` to the task. If the
task did not already declare `require-review-<x>`, that requirement SHALL be
added too, so the gate is both required and satisfied.

#### Scenario: Approve code review

- **WHEN** the code gate is approved on a task
- **THEN** the task carries `approved-review-code` (and `require-review-code`)

### Requirement: Idempotent

Approving a gate that is already approved SHALL be a no-op that reports the
gate is already approved.

#### Scenario: Already approved

- **WHEN** an already-approved gate is approved again
- **THEN** nothing changes and it is reported as already approved

### Requirement: Gates block merge

Merge SHALL be refused while any required review gate is unmet (see
action-merge).

#### Scenario: Unmet gate blocks merge

- **WHEN** a merge is attempted with an unmet gate
- **THEN** the merge is refused
