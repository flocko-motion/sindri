# View: Item Detail — delta

## MODIFIED Requirements

### Requirement: Detail sections

The item detail SHALL present, for a task Issue: metadata (id, status,
type, priority, linked spec, created/updated), review gates, the
assigned worker if any, associated PRs with their status, the full
description, acceptance criteria, and comments.

The TUI SHALL lay out the task detail as **two columns** side-by-side:

- **Left** (full-height, non-scrolling): metadata, review gates,
  worker, PRs — the formal data the eye scans.
- **Right** (scrollable): description, acceptance, comments — the
  free-text body.

The description and acceptance text SHALL be read from the structured
`td show --json` fields, NOT from the textual `td show` output, so the
metadata block is not echoed into the description body.

The CLI rendering of the same detail MAY remain single-column — both
interfaces serve the same content; the two-column rule is for the TUI
where horizontal space exists.

#### Scenario: Task with PRs and gates

- **WHEN** the detail of a task with PRs and review gates is shown
- **THEN** it includes the PR list with statuses and the gate states

#### Scenario: TUI task detail layout

- **WHEN** the TUI opens the detail of a task
- **THEN** the left column shows Metadata, Review Gates, Worker,
  Pull Requests
- **AND** the right column shows Description, Acceptance, Comments
- **AND** the right column is scrollable while the left is not

#### Scenario: Description has no echoed metadata

- **GIVEN** the underlying task has fields ID/Status/Type/Priority and
  a separate Description body
- **WHEN** the TUI renders the detail
- **THEN** the right pane's Description section contains only the body
  text — it does NOT repeat the ID/Status/Type/Priority already shown
  in the left pane's Metadata section
