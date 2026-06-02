## MODIFIED Requirements

### Requirement: Items as rows

The work list SHALL present each work item as a row showing its **type
indicator**, id, priority, last-updated time, status, and title, in the order
defined by the work-item refresh. Type SHALL be conveyed by a canonical glyph:
🐛 for bug, ✨ for feature, 🧹 for chore, 📦 for epic; ordinary tasks have no
glyph. Spec-only items SHALL keep the 📋 spec marker they already use.

#### Scenario: Bug row

- **WHEN** a task of type `bug` is listed
- **THEN** its row begins with 🐛 in the type column

#### Scenario: Feature row

- **WHEN** a task of type `feature` is listed
- **THEN** its row begins with ✨ in the type column

#### Scenario: Spec-only row

- **WHEN** a spec-only item is listed
- **THEN** its row is marked as a spec (e.g. "📋 spec") with its `os-……` id

## ADDED Requirements

### Requirement: Children indented under their parent

When a task carries a `parent_id`, the work list SHALL render it as a child of
that parent: the parent appears at its normal position; each child appears
immediately after, indented two spaces deeper than the parent, with a "↳"
prefix; a child's own children indent another two spaces. Siblings preserve
the same open → active → closed ordering used for top-level rows.

#### Scenario: Epic with two children

- **WHEN** the board contains an epic with two child tasks
- **THEN** the epic row appears at its normal position and the two child rows
  appear immediately after it, each indented two spaces with a "↳" prefix

#### Scenario: Grandchild

- **WHEN** a child task has a child of its own
- **THEN** the grandchild appears immediately after its parent, indented two
  more spaces (four deeper than the top-level row)

### Requirement: Blocked marker (deferred)

The work list SHALL eventually mark a task that is blocked by another with the
blocker's id (for example "⛓ blocked by td-……"). This requirement is captured
for completeness; implementation is deferred until td exposes the `depends_on`
relationship in its JSON output. Until then, the work list SHALL NOT pretend
to know that a task is blocked.

#### Scenario: td does not expose depends_on

- **WHEN** the work list is rendered against the current td JSON
- **THEN** no blocked marker is shown (the data is not yet available)
