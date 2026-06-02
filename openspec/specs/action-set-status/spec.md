# Action: Set Status

## Purpose

Defines changing a task's status, independent of any interface (the TUI cycles
it; other interfaces may set it directly). The transition rules are the same
everywhere.
## Requirements
### Requirement: Change status

Setting a task's status SHALL update it through the td adapter and SHALL accept
any status that td accepts (currently: `open`, `in_progress`, `in_review`,
`blocked`, `closed`). The TUI SHALL expose this as a **picker** modal opened
with the `s` key: it lists the supported statuses with the task's current one
pre-selected, lets the user navigate with the arrow keys, applies the choice
on Enter (or cancels on Esc), and reports the resulting transition.

#### Scenario: Picker shows current status

- **WHEN** the user opens the status picker for a task
- **THEN** the modal lists every supported status with the task's current one
  pre-selected

#### Scenario: Picker applies the choice

- **WHEN** the user selects a status and confirms
- **THEN** the new status is applied through the td adapter and the
  transition is reported to the user

#### Scenario: Picker cancelled

- **WHEN** the user dismisses the picker without confirming
- **THEN** the task's status is unchanged

