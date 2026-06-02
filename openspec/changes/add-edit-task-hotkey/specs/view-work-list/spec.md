# Work List View — delta

## ADDED Requirements

### Requirement: Edit a task from the list view

The work list SHALL bind `e` to open the edit-task modal for the task
under the cursor. The modal SHALL be pre-populated with the task's
current title, type, priority, and review-gate setting, and submit
SHALL update the task via `td update`. Non-task rows (spec-only, PR
sub-row, gate line) SHALL surface a visible "Edit: pick a task row
first" notification instead of opening anything.

#### Scenario: e on a task row

- **GIVEN** the cursor is on a task row for td-a
- **WHEN** the user presses `e`
- **THEN** the edit modal opens with td-a's title in the title field
- **AND** td-a's type and priority pre-selected
- **AND** the review checkbox reflects td-a's current label

#### Scenario: e on a spec-only row

- **GIVEN** the cursor is on a spec-only row
- **WHEN** the user presses `e`
- **THEN** the user sees "Edit: pick a task row first"

#### Scenario: e on a PR sub-row

- **GIVEN** the cursor is on a PR sub-row
- **WHEN** the user presses `e`
- **THEN** the user sees "Edit: pick a task row first"
