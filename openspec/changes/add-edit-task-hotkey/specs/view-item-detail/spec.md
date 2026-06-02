# Item Detail View — delta

## ADDED Requirements

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
