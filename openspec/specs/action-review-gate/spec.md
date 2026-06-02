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

### Requirement: Gate label format is identical across CLI and TUI

The system SHALL render every review gate in the user-facing format
`☑ review <type>` (satisfied) or `☐ review <type>` (unsatisfied),
where `<type>` is the gate's content suffix with internal dashes
replaced by spaces (e.g. `review-code` → `review code`). The
shortened-to-just-`<type>` form is forbidden — it strips the "review"
content and reads as too short.

The CLI and TUI SHALL call into a single shared formatter
(`render.GateLabel`) for one-gate display; the multi-gate joiner
(`render.Gates`) SHALL be defined as a thin loop over `GateLabel`.
Adding a new gate-rendering surface SHALL go through `GateLabel` so
the format cannot drift.

#### Scenario: Unapproved code review

- **GIVEN** a task with label `require-review-code`
- **WHEN** any interface renders the gate
- **THEN** it displays `☐ review code`

#### Scenario: Approved code review

- **GIVEN** a task with labels `require-review-code` and `approved-review-code`
- **WHEN** any interface renders the gate
- **THEN** it displays `☑ review code`

#### Scenario: Multi-word gate type

- **GIVEN** a task with label `require-review-security-design`
- **WHEN** any interface renders the gate
- **THEN** it displays `☐ review security design`

