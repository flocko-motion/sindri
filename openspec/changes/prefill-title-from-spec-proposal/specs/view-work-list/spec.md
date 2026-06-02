# Work List View — delta

## MODIFIED Requirements

### Requirement: Pre-link new-task creation from a spec row

The new-task hotkey (`n`) SHALL pre-link the modal to the spec at the
cursor when the cursor sits on a spec-only row: the modal SHALL show
the spec it will link to, SHALL pre-fill the title input from the
spec's proposal (first level-1 heading of `proposal.md`, or the slug
when the file is missing or has no H1), and the created task SHALL
carry the `spec:<name>` label without the user having to type it. From
any non-spec row, the hotkey SHALL open the unlinked modal as before.

#### Scenario: Pressing n on a spec-only row

- **GIVEN** the cursor is on the spec-only row for the spec `auth-refactor`
- **WHEN** the user presses `n`
- **THEN** the new-task modal opens with a "Linked to spec: 📄 auth-refactor"
  line at the top
- **AND** submitting the modal creates a task with the `spec:auth-refactor`
  label

#### Scenario: Title pre-fills from the spec proposal

- **GIVEN** the cursor is on the spec-only row for spec X
- **AND** `openspec/changes/X/proposal.md` exists with a level-1 heading
  "Roll out OAuth login provider"
- **WHEN** the user presses `n`
- **THEN** the title input is pre-filled with "Roll out OAuth login provider"
- **AND** the user can edit the title before submitting

#### Scenario: Proposal title falls back to the slug

- **GIVEN** the cursor is on the spec-only row for spec `csv-export`
- **AND** the proposal file is missing or has no level-1 heading
- **WHEN** the user presses `n`
- **THEN** the title input is pre-filled with "csv-export" (the slug)

#### Scenario: Pressing n on a task row

- **GIVEN** the cursor is on a task row (not a spec-only row)
- **WHEN** the user presses `n`
- **THEN** the new-task modal opens with no linked spec, and the created
  task has no `spec:` label
