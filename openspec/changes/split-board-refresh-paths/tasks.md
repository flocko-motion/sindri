## 1. Board package

- [x] 1.1 Split into `LoadTasks(root) ([]issue.Task, error)`, `LoadSpecs(root) []issue.Spec`, `LoadWorkers(root) []worker.Worker`, `LoadPRs(root) map[string][]issue.PR`, plus `WorkerByID(workers)` helper
- [x] 1.2 `List(root)` stays as a convenience wrapper that runs all four in parallel and `Assemble`s (CLI `task list` keeps using it)
- [x] 1.3 `LoadTasks` continues to apply `cachedParent`; cache warm flow unchanged

## 2. TUI data plumbing

- [x] 2.1 New `refreshTasksCmd` / `refreshSpecsCmd` / `refreshWorkersCmd` / `refreshPRsCmd` and a `refreshAllCmd` batch
- [x] 2.2 New `tasksRefreshedMsg{tasks, manual}` / `specsRefreshedMsg` / `workersRefreshedMsg` / `prsRefreshedMsg`
- [x] 2.3 Each cmd calls its loader and emits the matching message

## 3. Model + Update

- [x] 3.1 `Model.boardData` holds the current per-source snapshot (tasks/specs/workerByID/prsByID)
- [x] 3.2 `Init` returns `tea.Batch(refreshAllCmd, tickCmd, warmCacheCmd)`
- [x] 3.3 `tickCmd` re-dispatches `refreshAllCmd` + the next tick
- [x] 3.4 Each msg handler patches one field of `boardData`, calls `reassembleIssues` (runs `issue.Assemble` + `rebuildBacklog` + refreshes the detail pane); `tasksRefreshedMsg` sets `m.loaded = true`
- [x] 3.5 Legacy `refreshMsg` and `detectChanges` removed; their PR-created/status-transition notifications will land on the per-source PR handler when we revisit (intentionally deferred to keep this change tight)

## 4. Mutation hot path

- [x] 4.1 `movedMsg` and `statusChangedMsg` handlers dispatch only `refreshTasksCmd` — podman/openspec untouched
- [x] 4.2 `cacheWarmedMsg` dispatches `refreshTasksCmd` (parent_id cache only affects tasks)
- [x] 4.3 `actionResultMsg` keeps the full `refreshAllCmd` (approve/merge/reject can change PRs and workers)
- [x] 4.4 Manual `r` key dispatches `refreshAllCmd` with `manual=true` (still shows the `Refreshed — N tasks` toast)

## 5. Workers panel

- [x] 5.1 `workersRefreshedMsg` writes both `m.workers` and `m.boardData.workerByID`
- [x] 5.2 The "Loading workers…" placeholder stays until that message lands

## 6. Replay engine + goldens

- [x] 6.1 Replay still sets `m.issues` / `m.workers` directly; goldens unchanged

## 7. Validation

- [x] 7.1 `openspec validate split-board-refresh-paths --strict` passes
- [x] 7.2 `go build ./... && go test ./...` all green
- [x] 7.3 `sindri lint all` passes
- [x] 7.4 Wall-time on `/r/sindri` (89 tasks, 1 spec, 2 workers, 47 PRs):
       LoadTasks 175ms · LoadPRs 93ms · LoadSpecs 712ms · LoadWorkers 913ms.
       First paint = 175ms; post-mutation = 175ms.
