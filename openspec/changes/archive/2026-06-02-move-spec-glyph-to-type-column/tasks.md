# Tasks

## 1. Shared type-column builder

- [x] 1.1 `render.TypeColumn(iss)` is the single shared source of truth for the leftmost column
- [x] 1.2 Returns the type icon for task rows; 📄 for spec-only rows; depth indent + "↳ " prefix on children
- [x] 1.3 TUI's `typePrefix` becomes a thin wrapper

## 2. CLI table calls the shared builder

- [x] 2.1 `cmd/sindri/task.go` row builder drops its open-coded type cell
- [x] 2.2 Spec-only rows in `sindri task list` show 📄 in the TYPE column

## 3. Status column stops carrying the kind glyph

- [x] 3.1 `render.IssueStatus` returns "spec" / "spec X/Y" without the 📄 prefix
- [x] 3.2 Updated goldens show "spec" (no glyph) in the status column

## 4. Spec update

- [x] 4.1 `view-work-list` scenario for the spec-only row says 📄 is in the type column, not in the status column
- [x] 4.2 `openspec validate move-spec-glyph-to-type-column --strict` passes

## 5. Visual verification

- [x] 5.1 TUI goldens regenerated; spec row shows 📄 in the leftmost column, "spec" in the status column
- [x] 5.2 CLI table builds & shares the same `render.TypeColumn` function (verified via build + identical-input contract)
- [x] 5.3 `go test ./...` and `sindri lint all` pass
