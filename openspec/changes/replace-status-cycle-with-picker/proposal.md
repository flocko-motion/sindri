## Why

`td-4fbcca`: the TUI's `s` hotkey cycles a task's status between `open` and
`in_progress`, which sometimes silently *sets* it back to `open` (the wrong
move) and refuses to touch any other status. The current spec endorses that
cycle; both the spec and the implementation are the wrong UX.

## What Changes

- Replace the cycle-only status action with a status **picker**: pressing `s`
  on a task in the TUI SHALL open a modal listing every td status; selecting
  one applies it via the td adapter. The picker reflects the task's current
  status.
- The picker supports the full td status set: `open`, `in_progress`,
  `in_review`, `blocked`, `closed`.
- **BREAKING:** the action-set-status spec no longer requires a cycle. The
  TUI binding `s` changes its meaning from "cycle" to "open picker".

## Capabilities

### New Capabilities

- (none — this changes the existing `action-set-status` capability.)

### Modified Capabilities

- `action-set-status`: replace the cycle requirement with a picker-based
  requirement that admits the full td status set.

## Impact

- `internal/tui/actions.go` — replace `cycleTaskStatus` with a picker flow
  (new model fields `pickingStatus` / `statusOptions` / `statusCursor`, plus
  `updateStatusPick` handler and `setTaskStatus(s)` command).
- `internal/tui/tui.go` — wire the `s` key to open the picker and dispatch
  picker input.
- `internal/tui/testdata/frames/` — new golden `status-pick` and regenerated
  goldens for any state the picker affects.
- No CLI change.
