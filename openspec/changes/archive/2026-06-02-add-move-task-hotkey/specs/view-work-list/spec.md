## ADDED Requirements

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
