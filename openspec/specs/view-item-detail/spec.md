# View: Item Detail

## Purpose

Defines the full detail view of a single board item (an Issue, or a PR within
it), independent of any interface. The CLI (`task view` / `pr info`) and the TUI
detail pane are renderings of this one definition.

## Requirements

### Requirement: Detail sections

The item detail SHALL present, for a task Issue: metadata (id, status, type,
priority, linked spec, created/updated), the full description, review gates,
the assigned worker if any, associated PRs with their status, and comments.

#### Scenario: Task with PRs and gates

- **WHEN** the detail of a task with PRs and review gates is shown
- **THEN** it includes the PR list with statuses and the gate states

### Requirement: Spec-only detail

For a spec-only Issue, the detail SHALL show the spec name, its id, and its
task-checklist progress, and indicate that no task exists yet.

#### Scenario: Spec with no task

- **WHEN** the detail of a spec-only Issue is shown
- **THEN** it shows the spec and its progress, marked as needing a task

### Requirement: Field parity across interfaces

Every interface that shows item detail SHALL present the same fields for the
same item, differing only in layout. Styling SHALL come from the rendering
module.

#### Scenario: CLI and TUI agree

- **WHEN** the CLI and the TUI both show the same item's detail
- **THEN** they present the same fields
