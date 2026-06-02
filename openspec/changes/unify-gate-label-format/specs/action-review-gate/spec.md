# Action: Review Gate — delta

## ADDED Requirements

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
