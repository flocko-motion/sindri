# Work List View — delta

## ADDED Requirements

### Requirement: n picks up an auto-parent from the cursor row

The `n` (new task) hotkey SHALL pass the cursor row's task ID and
type to the create-task modal so the create-task action's
auto-parent rule (see capability `action-create-task`) can apply.
The modal SHALL render a visible "Auto-parent: td-XXX (cursor on
<type>)" line whenever the rule fires for the currently selected
type, and SHALL hide that line otherwise. Changing the type
selector in the modal SHALL recompute the preview live.

#### Scenario: n on an epic row shows the auto-parent line

- **GIVEN** the cursor sits on an epic row (td-eee)
- **WHEN** the user presses `n`
- **THEN** the modal shows "Auto-parent: td-eee (cursor on epic)"
- **AND** submitting creates a task whose parent_id is td-eee
  (assuming the default type `task` is unchanged)

#### Scenario: n on a feature row shows the auto-parent line

- **GIVEN** the cursor sits on a feature row (td-fff)
- **WHEN** the user presses `n`
- **THEN** the modal shows "Auto-parent: td-fff (cursor on feature)"

#### Scenario: n on a bug row hides the auto-parent line

- **GIVEN** the cursor sits on a bug row
- **WHEN** the user presses `n`
- **THEN** the modal does NOT show an Auto-parent line

#### Scenario: Toggling type to epic hides the preview live

- **GIVEN** the modal opened with the cursor on an epic
- **AND** the type selector starts at `task` (auto-parent shown)
- **WHEN** the user switches type to `epic`
- **THEN** the Auto-parent line disappears
