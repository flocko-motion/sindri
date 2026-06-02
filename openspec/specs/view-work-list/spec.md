# Work List View

## Purpose

Defines the work list — the primary view of all work items — independent of any
interface. The CLI (`sindri task list`) and the TUI backlog are two renderings
of this one definition; any future UI renders the same.
## Requirements
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

`a` SHALL approve the PR of the task at the cursor; `x` SHALL reject the
task at the cursor — OR, when the cursor is on a spec-only row, `x` SHALL
open the abandon-spec confirm dialog instead (the same key, the row kind
picks the verb). From any other row, `x` is the existing reject flow.
Every failure mode SHALL surface a visible notification.

#### Scenario: Approve a task row that has a PR

- **GIVEN** the cursor is on a task row whose task has at least one PR
- **WHEN** the user presses `a`
- **THEN** the PR is approved

#### Scenario: Approve a row with no PR

- **GIVEN** the cursor is on a task row whose task has no PR
- **WHEN** the user presses `a`
- **THEN** the user sees "Approve: this task has no PR yet"

#### Scenario: Reject a task row

- **GIVEN** the cursor is on a task row
- **WHEN** the user presses `x`
- **THEN** the reject-reason input opens

#### Scenario: x on a spec-only row opens abandon

- **GIVEN** the cursor is on a spec-only row for spec X
- **WHEN** the user presses `x`
- **THEN** the bottom bar shows the abandon-spec confirm — "Abandon spec
  X? Deletes the change folder and closes its linked open tasks. (y/n)"
- **AND** `y` runs the abandon (closes linked open tasks, deletes the
  change folder); `n` (or any other key) cancels with no side effect

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

### Requirement: Drift warning for tasks whose spec is gone

The work list SHALL render an `⚠ spec archived` warning (red, bold) in
the status column of any open task whose `spec:<name>` label points at a
spec that is no longer an active proposal — same warning style as the
orphan-in_progress marker. The warning SHALL be suppressed once the task
itself is closed, since closed is the steady state.

#### Scenario: Open task with archived spec

- **GIVEN** task td-a is open and carries `spec:X`
- **AND** spec X is archived
- **WHEN** the work list renders td-a
- **THEN** its status column reads `⚠ spec archived`

#### Scenario: Closed task with archived spec

- **GIVEN** task td-a is closed and carries `spec:X`
- **AND** spec X is archived
- **WHEN** the work list renders td-a
- **THEN** the warning is suppressed and the status column shows the
  task's regular closed status

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

### Requirement: Help bar lists every list-view binding

The work-list view SHALL render every action binding the list view
accepts as a `key:label` entry in the top help bar, so the help bar is
the visible inventory of what the user can do. Adding a new binding to
the list view SHALL include the corresponding help-bar entry.

#### Scenario: e:edit appears in the help bar

- **GIVEN** the list view binds `e` to "edit task"
- **WHEN** the help bar renders
- **THEN** the help bar contains the entry `e:edit`

#### Scenario: Adding a new binding without a help-bar entry is a regression

- **WHEN** the list view binds a new key K to some action
- **THEN** the help bar SHALL contain a `K:<label>` entry for it
- **AND** the regression test (golden capture of the help bar) SHALL
  refuse to pass without the entry

