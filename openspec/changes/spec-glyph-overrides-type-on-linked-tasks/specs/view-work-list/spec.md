# Work List View — delta

## MODIFIED Requirements

### Requirement: Items as rows

The work list SHALL present each work item as a row showing its **type
indicator**, id, priority, last-updated time, status, and title. The
type indicator SHALL follow the row's dominant identity:

- Issues paired with an active openspec change (spec-only OR
  task-linked) SHALL render 📄 in the type column. The 📄 SHALL
  replace the type glyph the row would otherwise carry.
- Plain (unlinked) tasks SHALL render their type glyph: 🐛 bug,
  ✨ feature, 🧹 chore, 📦 epic; ordinary tasks have no glyph.
- An orphan-linked task — one whose `spec:<name>` label points at a
  spec that is not an active proposal — SHALL render its type
  glyph (the spec is gone; the orphan-drift warning lives in the
  status column).

The linked-task title SHALL read `<spec-name> · <task-title>` (no
leading 📄), since the type column already carries the spec marker.

The priority and last-updated columns SHALL be visual-cell padded to
fixed widths (priority: 2 cells, timestamp: 14 cells) so the status
and title columns line up between task rows (which fill both fields)
and spec-only rows (which leave both empty).

The status column for a spec-only row SHALL show only the textual
status — "spec" or "spec X/Y" — with no kind glyph; the 📄 lives in
the type column.

#### Scenario: Bug row

- **WHEN** a task of type `bug` is listed without a spec link
- **THEN** its row begins with 🐛 in the type column

#### Scenario: Feature row

- **WHEN** a task of type `feature` is listed without a spec link
- **THEN** its row begins with ✨ in the type column

#### Scenario: Spec-only row

- **WHEN** a spec-only item is listed
- **THEN** the type column shows 📄
- **AND** the status column shows "spec" or "spec X/Y" with no glyph
- **AND** the id column shows the `os-……` id

#### Scenario: Task linked to an active spec

- **WHEN** a task carrying `spec:X` is listed and spec X is an active proposal
- **THEN** the type column shows 📄 (not the task's type glyph)
- **AND** the title reads `X · <task title>` with no leading 📄

#### Scenario: Task whose spec is archived (orphan)

- **WHEN** a task carrying `spec:X` is listed but spec X is archived or missing
- **THEN** the type column shows the task's type glyph (not 📄)
- **AND** the orphan-drift warning is surfaced in the status column

#### Scenario: Columns align between task and spec rows

- **GIVEN** a task row carrying a priority and a timestamp
- **AND** a spec-only row carrying neither
- **WHEN** both are listed
- **THEN** the status column starts at the same screen column on both rows
- **AND** the title column starts at the same screen column on both rows
