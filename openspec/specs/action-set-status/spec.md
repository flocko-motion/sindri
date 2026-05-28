# Action: Set Status

## Purpose

Defines changing a task's status, independent of any interface (the TUI cycles
it; other interfaces may set it directly). The transition rules are the same
everywhere.

## Requirements

### Requirement: Change status

Setting a task's status SHALL update it through the td adapter. A cycle action
SHALL move open → in_progress → open; statuses outside that cycle (review,
closed) SHALL NOT be changed by the cycle and SHALL report why.

#### Scenario: Cycle from open

- **WHEN** the status of an open task is cycled
- **THEN** it becomes in_progress

#### Scenario: Cycle a closed task

- **WHEN** the status of a closed task is cycled
- **THEN** it is left unchanged and the reason is reported
