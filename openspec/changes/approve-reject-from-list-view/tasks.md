## 1. Cursor helpers

- [x] 1.1 `cursorTaskAndPR()` returns `(taskID, prID string)` for the task at the backlog cursor
- [x] 1.2 Returns empty strings on non-task rows so callers branch with a clear notify

## 2. Detail-view silent fallthrough

- [x] 2.1 `a` (Approve) in detail view notifies "Approve: this task has no PR yet" when `m.detail.prIDs` is empty
- [x] 2.2 `m` (Merge) in detail view notifies "Merge: this task has no PR yet" when no PR

## 3. List-view bindings

- [x] 3.1 `a` in list view: resolves `cursorTaskAndPR()`; on success populates `m.detail` and dispatches `approvePR`; on no-PR or non-task surfaces a notify
- [x] 3.2 `x` in list view: resolves cursor task, populates `m.detail`, enters the reject-reason input mode (same setup as the detail-view `x`); falls back to `RejectTask` if no PR
- [x] 3.3 Every failure mode surfaces a visible notification

## 4. Replay + goldens

- [x] 4.1 Script step: cursor on td-aaaaaa (open, no PR), press `a`, capture `list-approve-no-pr` — bottom bar reads "Approve: this task has no PR yet"
- [x] 4.2 Wired into `TestReplayGoldens`

## 5. Validation

- [x] 5.1 `openspec validate approve-reject-from-list-view --strict` passes
- [x] 5.2 `go build ./... && go test ./...` all green
- [x] 5.3 `sindri lint all` passes
