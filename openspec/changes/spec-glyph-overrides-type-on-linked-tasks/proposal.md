# 📄 in the type column replaces the type icon for spec-linked tasks

## Why

`move-spec-glyph-to-type-column` put 📄 in the type column for spec-only
rows but kept the task's type icon (🪲/🔧/🔩/🧹/📦) on rows where a task
links to a spec, leaving 📄 only in the title prefix. On review:

> the document emoji should be displayed *instead* of the issue type
> emoji for issues with a linked openspec

A row's dominant identity, when it's paired with an active spec, is
*spec* — that's the contract the work list is reporting on. Showing the
type glyph there hides that fact and also creates two 📄's per row
(type column and title prefix) on every linked task.

## What Changes

- `render.TypeColumn` returns 📄 for *any* issue paired with an active
  openspec change — both spec-only AND task-linked. Plain (unlinked)
  tasks still render their type glyph.
- `issue.Title` drops the leading "📄 " from the linked-task title
  prefix; the title becomes `<spec-name> · <task-title>` (the 📄 now
  lives only in the type column, no duplication).
- Orphan-linked tasks (label points at an archived/missing spec) keep
  their type glyph — the spec is gone, so falling back to the type
  glyph keeps the row's kind identifiable. The orphan drift is already
  surfaced separately in the status column (`⚠ spec archived`).

## Impact

- Affected specs: `view-work-list` — the "Items as rows" requirement
  needs the rule for linked tasks; the spec-only scenario is
  unchanged.
- Affected code: `internal/render/render.go`, `internal/issue/issue.go`.
- Goldens regenerated: `list-default` now shows 📄 in the type column
  for `td-bbbbbb` (the spec-linked row) and its title reads
  `csv-export · add CSV export` (no leading 📄).
