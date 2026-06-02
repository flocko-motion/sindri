## Why

`td-b185e2`: there is no way to re-parent a task from the TUI today — the only
path is dropping to `td update <id> --parent <new>` in a shell. The user wants
to move items in the tree from the work list directly: pick the item, navigate
to the desired spot, and commit it as a sibling or a child with a single key.

## What Changes

- Add a **move mode** to the work list:
  - `m` (in the list view, on a task row) marks that task as "in movement" by
    highlighting it with a red background. The cursor is then free to navigate
    to any other row.
  - `h` commits the move as a **sibling** of the row under the cursor (the
    moving task's `parent_id` becomes the target's `parent_id`).
  - `l` commits the move as a **child** of the row under the cursor (the
    moving task's `parent_id` becomes the target's `id`).
  - `esc` cancels move mode.
  - Pressing `m` on a non-task row (spec-only, PR sub-row, workers panel)
    surfaces a visible notification instead of silently doing nothing.
  - The move is refused if target == source, if target is a descendant of
    source (would create a cycle), or if td rejects the parent change.
- Add `td.SetParent(root, id, parentID)` so the rest of the codebase has one
  authoritative entry point for the `td update --parent` call.
- After a successful move, the board refreshes and the parent-id cache is
  updated so the hierarchy redraws immediately.

## Capabilities

### New Capabilities

- (none — extends `view-work-list`.)

### Modified Capabilities

- `view-work-list`: add a "Move task to a different hierarchical position"
  requirement covering the modal, the visible highlight, the h/l/esc
  bindings, and the cycle/self-target rejection.

## Impact

- `internal/adapter/td/td.go` — add `SetParent`.
- `internal/tui/tui.go` — model gains `moving bool` + `movingTaskID string`;
  global dispatcher routes h/l/esc through `updateMoveMode` while `moving`.
- `internal/tui/actions.go` — add `enterMoveMode`, `cancelMove`, `applyMove`,
  `setTaskParent`.
- `internal/tui/backlog.go` — `backlogRow` gains `isMoving bool`; renderer
  applies a red-background style to the marked row (cursor styling wins for
  the cursor's own row, just with the red body).
- `internal/tui/styles.go` — add the red background style.
- `internal/tui/replay_test.go` + `testdata/frames/` — new `list-move-active`
  and `list-move-applied` goldens.
- No CLI change.
