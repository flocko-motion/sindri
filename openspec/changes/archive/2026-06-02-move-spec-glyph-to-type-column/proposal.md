# Move the spec glyph to the type column

## Why

`align-spec-rows-and-link-create` switched the spec marker from 📋 to 📄 but
left it in the **status** column ("📄 spec"). On review that landed wrong:
every other row carries a glyph in the leftmost **type** column (🪲 bug,
🔧 feature, 🔩 task, 🧹 chore, 📦 epic), and the spec marker belongs in
that same column — both visually and semantically (spec is the row's
"type" when there's no task yet). Leaving the status column to carry
the kind glyph means the eye reads two glyph columns and neither lines up.

## What Changes

- `render.TypeColumn(iss)` is the single shared source of truth for the
  leftmost column across every interface. Tasks render their type icon;
  spec-only rows render 📄. The CLI table and the TUI list both call it.
- `render.IssueStatus(iss)` no longer prepends 📄 on spec rows — status is
  just "spec" or "spec X/Y". The status column stays purely textual.
- TUI: `typePrefix` becomes a thin wrapper around `render.TypeColumn`.
- CLI: `cmd/sindri/task.go` drops its open-coded type-cell builder and
  calls `render.TypeColumn` directly.

## Impact

- Affected specs: `view-work-list` — MODIFIED: the "Items as rows" spec
  scenarios for the spec-only row now state the 📄 lives in the type
  column, not the status column.
- Affected code: `internal/render/render.go`, `internal/tui/backlog.go`,
  `cmd/sindri/task.go`.
- Goldens regenerated. The earlier `create-spec-linked` golden is
  unaffected (modal text). The list-view goldens now show 📄 in the type
  column and a plain "spec X/Y" status.
