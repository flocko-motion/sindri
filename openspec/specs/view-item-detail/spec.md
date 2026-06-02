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

### Requirement: Edit a task from the detail view

The detail view SHALL bind `e` to open the edit-task modal for the
currently-open task detail. Opening the modal SHALL also close the
detail pane so the modal is the only thing on screen. For non-task
details (PR detail, worker detail) the binding SHALL surface a
visible "Edit: only applies to tasks" notification.

#### Scenario: e while viewing a task detail

- **GIVEN** the detail view is open for task td-a
- **WHEN** the user presses `e`
- **THEN** the edit modal opens pre-populated with td-a's fields
- **AND** the detail pane closes

#### Scenario: e while viewing a PR detail

- **GIVEN** the detail view is open for a PR
- **WHEN** the user presses `e`
- **THEN** the user sees "Edit: only applies to tasks"

#### Scenario: e while viewing a worker detail

- **GIVEN** the detail view is open for a worker
- **WHEN** the user presses `e`
- **THEN** the user sees "Edit: only applies to tasks"

