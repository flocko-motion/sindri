# Tasks

## 1. Core

- [x] 1.1 `workflow.Engine.ReopenTask(project, id)`: refuses a `gh-`/`os-` id
      (`task.IsOwned`), naming where its status actually lives; refuses a task
      that is not closed rather than silently no-oping a typo'd id.
- [x] 1.2 The status flip goes through `SetStatus`, never `store.SetOwnedStatus`
      directly — the port boundary `onehome_test.go` holds the line on — then
      `RefreshTask` so the cached row a listing reads agrees with the owned one.
- [x] 1.3 Priority is untouched: no code path here writes it, which is what
      leaves a still-rated task immediately claimable again.

## 2. The reason, and where it lands

- [x] 2.1 `Hub.ReopenTask(project, id, author, reason)` is the one funnel: it
      refuses an empty reason, calls `workflow.Engine.ReopenTask`, then records
      `reason` as a comment on the task via `comments.Add`, attributed to
      `author`.
- [x] 2.2 Both callers below go through it, so the reason is required and
      recorded exactly once however it was asked for.

## 3. Exposed to the planner and the host, never a worker

- [x] 3.1 Agent verb `reopen-task <id> <reason...>`, registered `Roles:
      ["planner"]` only.
- [x] 3.2 CLI: `sindri task reopen <id> <reason...>`.
- [x] 3.3 TUI: `O` on a closed row on the Tasks tab (`taskReopenable` gates it
      to `Status == "closed"`; the hub, not the front-end, is what actually
      knows whether the id is sindri's to reopen).
- [x] 3.4 Both front-ends tell the caller when the task still carries a
      priority, since that alone makes it immediately claimable.

## 4. Wire

- [x] 4.1 `POST /task/reopen`, reusing `RejectReq{ID, Feedback}` — no new wire
      type.
- [x] 4.2 `client.HTTP.ReopenTask(id, reason)`.

## 5. Spec

- [x] 5.1 `05-workflow`: "The task lifecycle" gains the reopen path and states
      what it does NOT do (no new approval; priority stands).
- [x] 5.2 `05-workflow`: new requirement "Reopening a closed task" — scope
      (sindri-owned only), the required reason recorded as a comment, and who
      may invoke it.

## 6. Verify

- [x] 6.1 `workflow`: `TestReopenTaskRestoresAClosedOwnedTask`,
      `TestReopenTaskRefusesATaskThatIsNotClosed`,
      `TestReopenTaskRefusesAnUnownedID`.
- [x] 6.2 `tui`: `TestOptionsReopensAClosedTaskOnTheTasksTab`,
      `TestOptionsDoesNotReopenAnOpenTask`, `TestTaskReopenable`.
- [x] 6.3 `make verify` and `openspec validate --all` pass.
