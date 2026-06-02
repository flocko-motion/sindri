## Why

`td-bbc61b`: board.List today is monolithic — it fetches td tasks, openspec
changes, podman workers, and the PR store all together, then emits one
refreshMsg only when every source has finished. So the user waits on the
slowest (podman, ~1.5s) even just to see the task list. After a td edit, the
same monolithic refresh runs again to confirm — re-asking podman and openspec
about state that didn't change. As the user put it: *"we should be able to
just upload td after we modified a td item — we don't need to refresh podman
workers after editing a td item"*, and *"just load td first, then hot update
whenever new info comes in — we have an in-memory state for all items, so we
have something to display."*

## What Changes

- Replace the single `board.List` refresh path with **per-source loaders** that
  each emit their own message. The Model holds a `boardData` snapshot
  (tasks, specs, workerByID, prsByID) that each loader updates as it lands;
  `issue.Assemble` is re-run after every update.
- The TUI dispatches all four loaders in parallel on `Init` (and on tick /
  manual refresh). Tasks usually land first (~0.3s) and the list paints
  immediately; specs / PRs / workers enrich the rows when they arrive.
- After a td mutation (move, status pick, comment, …), only the **td loader**
  is dispatched in the background — podman and openspec are not contacted.
  The optimistic local update already showed the change; the td loader just
  confirms.
- `worker.List` is no longer on the foreground load path for `board.List`
  consumers; the workers loader runs alongside the others, and the workers
  panel shows "Loading workers…" until its message lands. Same pattern as the
  existing `parent_id` cache + `WarmParentCache` warmer.

## Capabilities

### New Capabilities

- (none — restructures 02-board's refresh.)

### Modified Capabilities

- `02-board`: replace the "Single refresh path" requirement with an
  "Incremental refresh" requirement covering per-source loaders, in-memory
  Model state, and the td-only post-mutation path.

## Impact

- `internal/board/board.go` — split `List` into per-source loaders
  (`LoadTasks`, `LoadSpecs`, `LoadWorkers`, `LoadPRs`); `List` stays as a
  convenience wrapper for callers that want all-at-once (CLI `task list`).
- `internal/tui/data.go` — new `refreshTasksCmd` / `refreshSpecsCmd` /
  `refreshWorkersCmd` / `refreshPRsCmd` and their messages.
- `internal/tui/tui.go` — `Model` gains `boardData`; `Init` and `tickCmd` fan
  out all four loaders; each msg handler updates one field of `boardData`,
  re-Assembles `m.issues`, rebuilds the backlog. `loaded = true` on the first
  tasks msg, so the placeholder clears as soon as something is shown.
- Mutation handlers (`movedMsg`, `statusChangedMsg`) dispatch only
  `refreshTasksCmd` instead of the full `refreshData`.
- No CLI change (CLI keeps using `board.List`).
- Replay engine unaffected: fixtures still set `m.issues` directly.
