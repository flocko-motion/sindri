# action-edit-task Specification

## Purpose
TBD - created by archiving change add-edit-task-hotkey. Update Purpose after archive.
## Requirements
### Requirement: Edit a task's fields

The system SHALL support editing an existing task's title, type, priority,
description, and labels independent of any interface. Edits SHALL go
through the td adapter (`td update`), and SHALL skip fields the caller
didn't change so the unchanged columns are not clobbered.

#### Scenario: Change priority only

- **GIVEN** an existing task td-a with title "fix login redirect"
- **WHEN** an edit submits only a new priority
- **THEN** the task's priority is updated
- **AND** its title, type, description, and labels are unchanged

#### Scenario: Change title

- **WHEN** an edit submits a new title
- **THEN** the task's title is updated via `td update --title`

### Requirement: Review-gate toggle preserves other labels

Editing the review-gate checkbox SHALL only add or remove the
`require-review-code` label. Other labels carried by the task —
`spec:<name>`, `approved-review-*`, custom labels — SHALL be preserved
across the edit so the toggle cannot silently drop them.

#### Scenario: Toggle review off on a task carrying spec link

- **GIVEN** task td-a carries labels `[spec:foo, require-review-code]`
- **WHEN** an edit submits with review unchecked
- **THEN** td-a's labels become `[spec:foo]`
- **AND** the `spec:foo` label is intact

#### Scenario: Toggle review on without altering approval labels

- **GIVEN** task td-a carries labels `[approved-review-code]`
- **WHEN** an edit submits with review checked
- **THEN** td-a's labels become `[approved-review-code, require-review-code]`

### Requirement: Edits do not enforce the 15-char minimum

The 15-character minimum-title rule SHALL apply only to task creation.
Editing an existing task SHALL accept any non-empty title — tasks
created via the `td` CLI or imported from elsewhere may have shorter
titles, and forcing the user to lengthen them just to change another
field would be obnoxious.

#### Scenario: Edit a 12-char title

- **GIVEN** an existing task whose title is "rename logger" (13 chars)
- **WHEN** an edit submits with the title unchanged and a new priority
- **THEN** the edit succeeds

