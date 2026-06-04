# Tasks

## 1. Adapter

- [x] 1.1 `td.CreateOpts` gains `Parent string`
- [x] 1.2 `td.Create` appends `--parent <id>` when set

## 2. Rule

- [x] 2.1 `resolveAutoParent(parentID, parentType, newType)` — pure helper
- [x] 2.2 `TestResolveAutoParent` covers all 14 branches (epic/feature/other × every newType, plus empty input)

## 3. Modal wiring

- [x] 3.1 `createTaskModel` gains `cursorParentID` + `cursorParentType` fields
- [x] 3.2 `newCreateTaskModel` accepts the cursor info from its caller
- [x] 3.3 Modal renders "Auto-parent: td-XXX (cursor on <type>)" when the rule fires for the current type selector
- [x] 3.4 Submit applies `resolveAutoParent` and passes `Parent` to `td.Create`
- [x] 3.5 Edit mode is unaffected (never re-parents)

## 4. Cursor helper

- [x] 4.1 `Model.cursorAutoParent()` returns `(taskID, taskType)` for the cursor's task row, or empty strings for ineligible rows (non-task, non-epic-or-feature)
- [x] 4.2 `n` handler in `tui.go` passes the helper's result into `newCreateTaskModel`

## 5. Spec + goldens

- [x] 5.1 `action-create-task` ADDS the auto-parent requirement
- [x] 5.2 `view-work-list` adds a scenario tying the rule to the `n` hotkey
- [x] 5.3 New goldens: `create-auto-parent-epic`, `create-auto-parent-feature`, `create-no-auto-parent-bug`
- [x] 5.4 `openspec validate auto-parent-on-create --strict` passes
- [x] 5.5 `go test ./...` + `sindri lint all` pass
