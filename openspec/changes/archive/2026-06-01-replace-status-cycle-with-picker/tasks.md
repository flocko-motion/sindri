## 1. Model + picker mode

- [x] 1.1 Add `pickingStatus bool`, `statusOptions []string`, `statusCursor int` to `Model`
- [x] 1.2 Define the canonical status set `{open, in_progress, in_review, blocked, closed}` in one place (`statusOptions()` in actions.go)
- [x] 1.3 On `s` (from task detail), populate `statusOptions`, set `statusCursor` to the task's current status, set `pickingStatus=true`

## 2. Update + commands

- [x] 2.1 Add `updateStatusPick(msg)` handling left/right/enter/esc (h/l accepted too)
- [x] 2.2 Replace `cycleTaskStatus` with `setTaskStatus(status)` (calls `td.SetStatus`)
- [x] 2.3 Dispatch picker input from `updateDetail` before the regular detail handler

## 3. View

- [x] 3.1 Render the picker as a bottom-bar pill row with brackets around the cursor option
- [x] 3.2 Help line surfaces the picker keys (`←/→ pick, enter apply, esc cancel`)

## 4. Replay + golden

- [x] 4.1 Open task detail in the script, press `s`, capture `status-pick`
- [x] 4.2 Move the cursor once and capture `status-pick-moved`
- [x] 4.3 Wire both into `TestReplayGoldens`

## 5. Validation

- [x] 5.1 `openspec validate replace-status-cycle-with-picker --strict` passes
- [x] 5.2 `go build ./... && go test ./...` all green
- [x] 5.3 `sindri lint all` passes
