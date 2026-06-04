# Tasks

## 1. Help bar

- [x] 1.1 Help moves to its own row below the title; grouped by ` · ` into nav / row actions / view actions
- [x] 1.2 Backlog help lists every list-view binding: j/k enter:open y n e a x s m c f r q
- [x] 1.3 Workers help drops the row-action keys (they only apply to tasks)
- [x] 1.4 `contentHeight` accounts for the extra row (`m.height - 5` instead of `-4`)
- [x] 1.5 `TestColumnFitsWidth` skips the help row (fixed chrome, same as the title bar)

## 2. Approve without PR

- [x] 2.1 `action.ApproveTask(root, taskID)` closes the task with reason "approved"
- [x] 2.2 List-view `a` handler routes through `approveTaskNoPR` when `cursorTaskAndPR` returns `prID == ""`
- [x] 2.3 Detail-view `a` handler routes through `approveTaskNoPR` when `m.detail.prIDs` is empty
- [x] 2.4 `approveTaskNoPR` returns `statusChangedMsg{newStatus: "closed"}` so the spec-lifecycle check fires

## 3. Specs + checks

- [x] 3.1 `view-work-list` "Help bar lists every list-view binding" requirement updated for the dedicated row + group structure
- [x] 3.2 `action-approve` MODIFIED: approve has a with-PR path AND a no-PR path that closes the task
- [x] 3.3 `openspec validate comprehensive-list-help-bar-and-approve-no-pr --strict` passes
- [x] 3.4 `go test ./...` + `sindri lint all` pass
- [x] 3.5 Goldens regenerated
