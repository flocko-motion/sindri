# Work List View — delta

## MODIFIED Requirements

### Requirement: Items as rows

The work list SHALL present each work item as a row showing its **type
indicator**, id, priority, last-updated time, status, and title, in the order
defined by the work-item refresh. Type SHALL be conveyed by a canonical glyph:
🐛 for bug, ✨ for feature, 🧹 for chore, 📦 for epic; ordinary tasks have no
glyph. Spec-only items SHALL keep the 📄 spec marker they already use.

The priority and last-updated columns SHALL be visual-cell padded to fixed
widths (priority: 2 cells, timestamp: 14 cells) so the status and title
columns line up between task rows (which fill both fields) and spec-only
rows (which leave both empty).

#### Scenario: Bug row

- **WHEN** a task of type `bug` is listed
- **THEN** its row begins with 🐛 in the type column

#### Scenario: Feature row

- **WHEN** a task of type `feature` is listed
- **THEN** its row begins with ✨ in the type column

#### Scenario: Spec-only row

- **WHEN** a spec-only item is listed
- **THEN** its row is marked as a spec (e.g. "📄 spec") with its `os-……` id

#### Scenario: Columns align between task and spec rows

- **GIVEN** a task row carrying a priority and a timestamp
- **AND** a spec-only row carrying neither
- **WHEN** both are listed
- **THEN** the status column starts at the same screen column on both rows
- **AND** the title column starts at the same screen column on both rows

## ADDED Requirements

### Requirement: Pre-link new-task creation from a spec row

The new-task hotkey (`n`) SHALL pre-link the modal to the spec at the
cursor when the cursor sits on a spec-only row: the modal SHALL show the
spec it will link to, and the created task SHALL carry the `spec:<name>`
label without the user having to type it. From any non-spec row, the
hotkey SHALL open the unlinked modal as before.

#### Scenario: Pressing n on a spec-only row

- **GIVEN** the cursor is on the spec-only row for the spec `auth-refactor`
- **WHEN** the user presses `n`
- **THEN** the new-task modal opens with a "Linked to spec: 📄 auth-refactor"
  line at the top
- **AND** submitting the modal creates a task with the `spec:auth-refactor`
  label

#### Scenario: Pressing n on a task row

- **GIVEN** the cursor is on a task row (not a spec-only row)
- **WHEN** the user presses `n`
- **THEN** the new-task modal opens with no linked spec, and the created
  task has no `spec:` label
