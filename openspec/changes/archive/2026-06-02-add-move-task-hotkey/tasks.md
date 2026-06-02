## 1. Adapter

- [x] 1.1 `td.SetParent(root, id, parentID string) error` calls `td update <id> --parent <parentID>` (empty string clears)

## 2. Model + state

- [x] 2.1 `moving bool` + `movingTaskID string` on `Model`
- [x] 2.2 list-view `h`/`l`/`esc` dispatch checks `moving` first; otherwise the existing navigation keeps working
- [x] 2.3 `taskAtCursor()` returns the target task's `(id, parentID)`

## 3. Move actions (internal/tui/actions.go)

- [x] 3.1 `enterMoveMode` sets the flag, stashes the id, refreshes the rows, and emits a "press h/l/esc" notification
- [x] 3.2 `applyMove(asChild bool)` chooses sibling vs child, refuses self / cycle / non-task target via visible notifications, then calls `setTaskParent`
- [x] 3.3 `setTaskParent(taskID, newParentID)` runs `td.SetParent`, updates `board.SetCachedParent`, and reports the outcome
- [x] 3.4 `cancelMove` clears the flag and the id

## 4. Rendering

- [x] 4.1 `backlogRow.isMoving` flag; `rebuildBacklog` sets it for the row whose `Task.ID == m.movingTaskID`
- [x] 4.2 `movingItemStyle` (red background, white foreground) in `styles.go`
- [x] 4.3 `renderBacklogList` paints the moving row red; cursor's `> ` still draws on top when it sits on the same row

## 5. Replay + goldens

- [x] 5.1 New script step: from the list view, press `m` while the cursor is on the in-progress task — capture `list-move-active` showing the row "in flight"
- [ ] 5.2 Skip: applied-move capture would actually shell out to `td.SetParent`; leaving the post-apply state for manual end-to-end verification
- [x] 5.3 Wired into `TestReplayGoldens`

## 6. Validation

- [x] 6.1 `openspec validate add-move-task-hotkey --strict` passes
- [x] 6.2 `go build ./... && go test ./...` all green
- [x] 6.3 `sindri lint all` passes
