## Why

`td-4a13d3`: on TUI startup the backlog and workers panels render their empty
placeholders ("No tasks or PRs", "No workers") for the second or two between
when the window first sizes itself and when the first refresh lands. That is a
silent and misleading state — it looks like sindri found nothing, then magically
finds tasks. The user should see "Loading tasks…" instead.

## What Changes

- The work-list and workers views SHALL distinguish three states: **loading**
  (before the first refresh applies), **empty** (refresh applied, nothing
  matches the current filter), and **populated**.
- During the loading state the panels show "Loading tasks…" / "Loading
  workers…" in place of the empty placeholders.
- The Model tracks first-refresh-applied via a new `loaded` flag; the replay
  engine offers a `LoadingState` fixture flag to capture the loading state
  deterministically.

## Capabilities

### New Capabilities

- (none — this modifies two existing capabilities.)

### Modified Capabilities

- `view-work-list`: add a "Loading state" requirement before the empty-state
  placeholder.
- `view-workers`: add a "Loading state" requirement before the empty-state
  placeholder.

## Impact

- `internal/tui/tui.go` — add `loaded bool` to `Model`; set it `true` on every
  `refreshMsg`.
- `internal/tui/backlog.go` and `internal/tui/workers.go` — accept a `loaded`
  flag from the View call and render the loading placeholder when `!loaded`.
- `internal/tui/replay.go` — `Fixture` gains `LoadingState bool`; when true the
  engine leaves `loaded` false so the loading state can be captured.
- `internal/tui/testdata/frames/` — new goldens `list-loading` and
  `workers-loading`.
- No CLI change.
