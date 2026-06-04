# Action: Create Task — delta

## ADDED Requirements

### Requirement: Auto-parent from the invocation cursor

The create-task action SHALL accept an optional parent suggestion
derived from the cursor row at the moment the modal opens. When the
cursor row is a task of type `epic`, the new task SHALL be created
as a child of that epic UNLESS its own type is also `epic`. When the
cursor row is a task of type `feature`, the new task SHALL be
created as a child of that feature UNLESS its own type is `epic` or
`feature`. For all other cursor rows (other task types, spec-only
rows, PR sub-rows, non-list views), no auto-parent SHALL be applied.

The auto-parent rule is create-only. Editing an existing task SHALL
NEVER re-parent it.

The UI surfacing the rule SHALL show the user the resolved parent
before submit so the auto-parent isn't silent — the user can change
the new task's type to opt out of the parenting.

#### Scenario: Epic + smaller type → child

- **GIVEN** the cursor sits on an epic td-eee
- **WHEN** the user creates a task of type `task`, `bug`, `feature`,
  or `chore`
- **THEN** the new task is created with `--parent td-eee`

#### Scenario: Epic + epic → no auto-parent

- **GIVEN** the cursor sits on an epic td-eee
- **WHEN** the user creates a task of type `epic`
- **THEN** the new task is created with no parent (epics are roots)

#### Scenario: Feature + smaller type → child

- **GIVEN** the cursor sits on a feature td-fff
- **WHEN** the user creates a task of type `task`, `bug`, or `chore`
- **THEN** the new task is created with `--parent td-fff`

#### Scenario: Feature + feature/epic → no auto-parent

- **GIVEN** the cursor sits on a feature
- **WHEN** the user creates a task of type `feature` or `epic`
- **THEN** the new task is created with no parent

#### Scenario: Cursor on bug/task/chore → no auto-parent

- **GIVEN** the cursor sits on a row of any type other than epic or
  feature
- **WHEN** the user creates a new task of any type
- **THEN** the new task is created with no parent

#### Scenario: Edit mode never re-parents

- **WHEN** the edit modal submits
- **THEN** the task's `parent_id` is left unchanged regardless of
  the auto-parent rule's would-be result
