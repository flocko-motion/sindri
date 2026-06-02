# Work List View — delta

## MODIFIED Requirements

### Requirement: Items as rows

The work list SHALL present each work item as a row showing its **type
indicator**, id, priority, last-updated time, status, and title, in the order
defined by the work-item refresh. Type SHALL be conveyed by a canonical glyph
in the leftmost column: 🐛 for bug, ✨ for feature, 🧹 for chore, 📦 for epic;
ordinary tasks have no glyph. Spec-only items SHALL render 📄 in that same
**type** column — never in the status column — so every row's kind reads off
the same column.

The priority and last-updated columns SHALL be visual-cell padded to fixed
widths (priority: 2 cells, timestamp: 14 cells) so the status and title
columns line up between task rows (which fill both fields) and spec-only
rows (which leave both empty).

The status column for a spec-only row SHALL show only the textual status —
"spec" or "spec X/Y" — with no kind glyph; the 📄 lives in the type column.

#### Scenario: Bug row

- **WHEN** a task of type `bug` is listed
- **THEN** its row begins with 🐛 in the type column

#### Scenario: Feature row

- **WHEN** a task of type `feature` is listed
- **THEN** its row begins with ✨ in the type column

#### Scenario: Spec-only row

- **WHEN** a spec-only item is listed
- **THEN** the type column shows 📄
- **AND** the status column shows "spec" or "spec X/Y" with no glyph
- **AND** the id column shows the `os-……` id

#### Scenario: Columns align between task and spec rows

- **GIVEN** a task row carrying a priority and a timestamp
- **AND** a spec-only row carrying neither
- **WHEN** both are listed
- **THEN** the status column starts at the same screen column on both rows
- **AND** the title column starts at the same screen column on both rows
