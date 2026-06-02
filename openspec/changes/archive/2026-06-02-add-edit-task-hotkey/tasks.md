# Tasks

## 1. Adapter

- [x] 1.1 `td.UpdateOpts` struct (Title, Type, Priority, Body, Labels — all optional)
- [x] 1.2 `td.Update(root, id, opts)` skips empty fields so partial updates don't clobber unchanged columns
- [x] 1.3 `--title` flag receives the title value directly; `-d` only when Body is set

## 2. Edit-mode modal

- [x] 2.1 `newEditTaskModel(root, t issue.Task)` pre-fills title, type, priority, review-checkbox from t
- [x] 2.2 `createTaskModel.editingID` + `origLabels` carry edit state
- [x] 2.3 Modal heading reads "Edit Task — <id>" when editing
- [x] 2.4 Submit branches on editingID: edit → `td.Update`; create → `td.Create`
- [x] 2.5 Edit preserves every non-review label (spec:..., approved-*, etc.) so the toggle never silently drops them
- [x] 2.6 Edit waives the 15-character min-title rule

## 3. Hotkey wiring

- [x] 3.1 `keys.EditTask` bound to `e`
- [x] 3.2 List view: `e` on a task row → `newEditTaskModel`; non-task row → visible notify
- [x] 3.3 Detail view: `e` for a task detail → opens edit modal, closes detail; non-task detail → visible notify
- [x] 3.4 `taskUpdatedMsg` handler refreshes the board and notifies "Task updated: <id>"

## 4. Golden + checks

- [x] 4.1 `edit-task` golden captures the pre-filled modal after `e` on td-aaaaaa
- [x] 4.2 `go test ./...` green; `sindri lint all` green
- [x] 4.3 `openspec validate add-edit-task-hotkey --strict` passes
