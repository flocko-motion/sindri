# Align spec rows; `n` on a spec row pre-links the new task

## Why

Two related papercuts on the work list:

1. Spec-only rows leave the priority and timestamp cells unpadded, so the title
   on a spec row lands ~16 cells to the left of every task row's title. The
   eye can't follow a vertical column down the list — spec rows look like a
   different layout.
2. Pressing `n` on a spec-only row opens the empty new-task modal with no
   pre-fill, even though the *point* of seeing the spec row is "this spec
   has no task — make one." The user has to remember the spec name and add
   the `spec:<name>` label manually after, or fall back to the CLI's
   `task new --spec`.

This change does both at once because the spec row's job is to invite that
exact gesture: see "(no task — needs planning)", press `n`, get a modal
that's already linked to the spec.

It also switches the spec marker glyph from 📋 (clipboard) to 📄 (document),
which reads as "spec document" rather than "to-do list."

## What Changes

- Pad the priority column to 2 cells and the timestamp column to 14 cells
  with cell-aware padding (`lipgloss.Width`-based), so spec rows align with
  task rows even though they have neither field.
- Replace 📋 with 📄 everywhere a spec is rendered: the work-list status cell
  for spec-only rows, and the title prefix used on tasks linked to a spec.
- The `n` (new task) hotkey, when fired with the cursor on a spec-only row,
  opens the modal pre-linked to that spec: a "Linked to spec: 📄 <name>"
  line at the top, and the resulting task carries the `spec:<name>` label.
  From any other row, `n` behaves as before.

## Impact

- Affected specs:
  - `view-work-list` — MODIFIED: spec row alignment, glyph; ADDED: pre-link
    new-task creation from a spec-only row.
  - `action-create-task` — MODIFIED: the optional spec link can now be set
    by the invocation context (cursor row), not only by an explicit flag.
- Affected code: `internal/render/render.go`, `internal/issue/issue.go`,
  `internal/tui/backlog.go`, `internal/tui/actions.go`,
  `internal/tui/create_task.go`, `internal/tui/tui.go`.
- Goldens regenerated; one new golden (`create-spec-linked`) added.
