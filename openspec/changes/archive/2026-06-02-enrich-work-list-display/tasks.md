## 1. Adapter + data

- [x] 1.1 `rawTask` gains `ParentID string` mapped from JSON `parent_id`; `toTask` copies it
- [x] 1.2 New `td.Enrich(root, []issue.Task)` calls `td show <id> --json` per task and fills fields td list strips (currently: parent_id); per-task failures log to stderr and skip
- [x] 1.3 `board.List` calls `td.Enrich` after the bulk list so every Issue carries parent_id

## 2. Issue model + board

- [x] 2.1 Add `Task.ParentID string`
- [x] 2.2 Add `Issue.Depth int`
- [x] 2.3 (Accessor not needed — direct field is sufficient and explicit.)
- [x] 2.4 Add `issue.ArrangeHierarchy([]Issue) []Issue` that reorders depth-first and stamps Depth; `Assemble` calls it at the end

## 3. Rendering

- [x] 3.1 Add `render.TaskTypeIcon(typ)` returning 🐛/✨/🧹/📦/"" for bug/feature/chore/epic/task
- [x] 3.2 `backlog.buildBacklogRows` prepends the type icon to the title (both plain and styled paths) and indents child rows `2*Depth` spaces with a `↳ ` prefix
- [x] 3.3 `cmd/sindri/task.go runTaskList` does the same

## 4. Replay engine + goldens

- [x] 4.1 Add `MockFixture()` — epic + feature/chore children + standalone bug + plain task
- [x] 4.2 `Replay` runs the fixture's Issues through `issue.ArrangeHierarchy` so fixture authors only set `ParentID`
- [x] 4.3 New golden `list-mock` in `TestReplayGoldens_Mock`; regenerated existing goldens that picked up the new type column

## 5. Mock td data

- [x] 5.1 Seed five demo tasks (`label:demo`) — an epic with two children plus a standalone bug and a plain task — so `sindri tui` shows hierarchy live

## 6. Validation

- [x] 6.1 `openspec validate enrich-work-list-display --strict` passes
- [x] 6.2 `go build ./... && go test ./...` all green
- [x] 6.3 `sindri lint all` passes
