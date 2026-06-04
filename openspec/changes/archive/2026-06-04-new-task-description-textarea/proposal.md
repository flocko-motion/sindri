# Description input is multi-line; ctrl+s submits

## Why

The new-task modal's description field was a single-line `textinput`
that scrolled horizontally once the text passed the row width — the
user described it as "glitching weirdly". Long descriptions are
exactly the case the field exists for, so silently breaking on them
defeats the purpose.

## What Changes

- Description input switches from `bubbles/textinput` to
  `bubbles/textarea`: wraps onto multiple lines, scrolls vertically
  via the textarea's own line buffer.
- The modal widens to 80 columns (was 60) so the textarea has room
  for ~67 characters per line.
- The textarea is 5 rows tall; the "Desc:" label sits on its own row
  so wrapped/continuation lines all start at the same column rather
  than the second row trying to land under the "Desc:  " indent.
- New `ctrl+s` shortcut submits the form from any field. The plain
  `enter` key still submits when the cursor is on the title (the
  most common case) but inserts a newline when the cursor is in the
  description textarea — otherwise pressing enter to start a new
  paragraph would create the task half-typed.
- The bottom help line now reads `tab:next field  h/l:select
  enter:create (in description: newline)  ctrl+s:submit  esc:cancel`
  so the dual `enter` behavior and the new submit shortcut are
  discoverable.
- The same textarea is used in the `e`-mode edit modal — its
  placeholder text just changes to "leave empty to keep current"
  because td.Update only writes the description when a body is
  supplied.

## Impact

- Affected spec: `action-create-task` — the existing "create a task"
  requirement gains a clause that the description input MUST be
  multi-line; a new requirement pins the dual `enter` behavior and
  the `ctrl+s` submit shortcut.
- Affected code: `internal/tui/create_task.go` (textarea swap,
  submit-shortcut split, modal width).
- Goldens regenerated: `create-spec-linked`, `edit-task` — the
  modal is wider and shows a multi-row description input.
