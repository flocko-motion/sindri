# Work List View

## Purpose

Defines the work list — the primary view of all work items — independent of any
interface. The CLI (`sindri task list`) and the TUI backlog are two renderings
of this one definition; any future UI renders the same.
## Requirements
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

### Requirement: Worker and orphan status

When a worker is assigned to an item, the row's status SHALL show the worker
instead of the raw status. An `in_progress` item with no worker SHALL be shown
as a warning (orphaned).

#### Scenario: Assigned worker

- **WHEN** a worker is working an item
- **THEN** the status cell shows that worker, not the bare "in_progress"

### Requirement: PRs and gates beneath their item

Each item's associated PRs and unmet/met review gates SHALL be shown beneath it,
visually subordinate to the item row.

#### Scenario: Item with a PR

- **WHEN** an item has an associated PR
- **THEN** the PR appears as a sub-row under the item with its status

### Requirement: Status filtering

The work list SHALL hide closed items by default and SHALL offer to show all
items, only open items, or only closed items.

#### Scenario: Default view

- **WHEN** the work list is shown with no filter
- **THEN** closed, approved, and merged items are omitted

### Requirement: Identical across interfaces

Every interface that renders the work list SHALL show the same items with the
same fields, differing only in presentation (table vs. scrolling viewport).

#### Scenario: CLI and TUI agree

- **WHEN** the CLI list and the TUI backlog are shown for the same project
- **THEN** they contain the same items with the same per-row fields

### Requirement: Loading state distinct from empty

Before the first board refresh has applied, the work list SHALL show a
"Loading tasks…" placeholder, distinct from the empty-state placeholder used
when a refresh has applied but no items match the current filter. The loading
placeholder SHALL replace any empty-state text in this window so the user is
never misled into thinking the board is empty before the data has arrived.

#### Scenario: First frame after startup

- **WHEN** the TUI is rendered after the window has sized itself but before
  the first refresh has applied
- **THEN** the work-list panel shows "Loading tasks…" instead of
  "No tasks or PRs"

#### Scenario: Refresh applied, board truly empty

- **WHEN** a refresh has applied and the filtered list contains no items
- **THEN** the work-list panel shows its empty-state placeholder ("No tasks
  or PRs"), not the loading text

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

### Requirement: Move task to a different hierarchical position

The work list SHALL let the user re-parent a task in the tree without leaving
the TUI. Pressing `m` on a task row in the backlog enters **move mode**: the
selected task is marked with a red background and the cursor is free to
navigate to any other row. From move mode, `h` commits the move as a sibling
of the row under the cursor (the moving task's `parent_id` becomes the
target's `parent_id`); `l` commits it as a child of the target (the moving
task's `parent_id` becomes the target's `id`); `esc` cancels. After a
successful move the board SHALL refresh so the hierarchy redraws with the
new layout. Pressing `m` on a non-task row SHALL surface a visible
notification rather than silently doing nothing.

#### Scenario: Mark a task for moving

- **WHEN** the user presses `m` while the cursor is on a task row
- **THEN** that task is marked with a red background and the cursor is free to
  move; the previous behaviour of `h` and `l` (collapse / expand) is
  suspended until the move is committed or cancelled

#### Scenario: Commit as sibling

- **WHEN** the user, in move mode, presses `h` while the cursor is on a
  different task row
- **THEN** the moving task's `parent_id` is set to the target task's
  `parent_id` (i.e. they become siblings), the board refreshes, and the new
  layout is shown

#### Scenario: Commit as child

- **WHEN** the user, in move mode, presses `l` while the cursor is on a
  different task row
- **THEN** the moving task's `parent_id` is set to the target task's `id`
  (the target becomes the new parent), the board refreshes, and the new
  layout is shown

#### Scenario: Cancel

- **WHEN** the user presses `esc` during move mode
- **THEN** move mode ends with no change and the row's red marking clears

#### Scenario: Refuse self or cycle

- **WHEN** the user attempts to make the moving task its own sibling/child of
  itself, or a child of one of its own descendants
- **THEN** the move is refused with a visible notification and move mode
  stays active

#### Scenario: Non-task row

- **WHEN** the user presses `m` on a non-task row (spec-only, PR sub-row, the
  workers panel)
- **THEN** a visible notification is shown ("Move: pick a task row first") and
  no move begins

### Requirement: Approve and reject from the list view

The work list SHALL let the user approve or reject a task without first
opening the detail view. `a` pressed in the list view, with the cursor on a
task row whose Issue has an open PR, SHALL approve that PR using the same
shared logic the detail view's `a` uses (the `action.Approve` path), and SHALL
update the row optimistically. `x` pressed in the list view, with the cursor
on a task row, SHALL enter the reject-reason input flow (same input flow the
detail view's `x` uses); on submission the task is rejected using
`action.Reject` for the cursor row's PR (or `action.RejectTask` if no PR
exists yet).

Every failure mode SHALL produce a visible notification — pressing `a` on a
non-task row, on a task with no PR, or on a PR that cannot be approved, SHALL
NOT silently do nothing. The detail view's existing `a` and `m` bindings,
which previously fell through silently when `m.detail.prIDs` was empty, SHALL
also surface a visible notification in that case.

#### Scenario: Approve from the list view

- **WHEN** the user presses `a` while the cursor is on a task row that has an
  associated open PR
- **THEN** that PR is approved through the shared action and the user sees a
  confirmation notification

#### Scenario: Reject from the list view

- **WHEN** the user presses `x` while the cursor is on a task row
- **THEN** the reject-reason input opens, and on submission the task is
  rejected with the typed reason via the shared action

#### Scenario: No PR yet

- **WHEN** the user presses `a` on a task that has no PR yet, in either the
  list or detail view
- **THEN** a notification "Approve: this task has no PR yet" is shown and the
  key never silently does nothing

#### Scenario: Non-task row

- **WHEN** the user presses `a` or `x` on a spec-only row, a PR sub-row, or
  the workers panel
- **THEN** a visible notification names the constraint ("Approve: pick a task
  row first" / "Reject: pick a task row first")

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

