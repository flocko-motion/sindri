# Two-column task detail view

## Why

The task detail view was a single column: Metadata, Description (which
echoed the `td show` text — re-rendering ID, status, type, priority on
top of its body), Review Gates, Worker, PRs, Comments — all stacked
vertically. Two problems:

1. The "Description" section dumped the full `td show` text and that
   text starts with ID/status/type/priority — the exact same fields
   already laid out as Metadata above. Visible duplication.
2. With everything stacked, the eye has to scroll to find anything;
   formal data and free-text body fight for the same screen space.

User:

> The layout of the details view of a task is shitty. We need two
> columns. Left is one full-height column with formal data
> (metadata, gates), right is a scrollable text pane with description.

## What Changes

- Task detail is laid out as two columns side-by-side:
  - **Left** (full-height, non-scrolling): Metadata, Review Gates,
    Worker, Pull Requests.
  - **Right** (scrollable viewport): Description, Acceptance criteria,
    Comments.
- The right pane reads from the structured `td show --json` fields
  (`description`, `acceptance`) — NOT the textual `td show` — so the
  metadata block doesn't get re-echoed into the description body.
- `td.Detail(root, id)` is the new adapter call that returns the
  structured description and acceptance fields together.
- Each column is sized to half the terminal width; section borders
  inside scale to match so they fill the column cleanly instead of
  leaving a wide trailing gap.
- Spec-only detail and the PR / worker details keep their existing
  single-column rendering — they have no separate text body.

## Impact

- Affected spec: `view-item-detail` — the task-detail requirement is
  rewritten with the two-column rule and the structured-fields rule.
- Affected code: `internal/adapter/td/td.go` (new `Detail`),
  `internal/tui/data.go` (new `fetchTaskAcceptance` seam,
  `fetchTaskDetail` rerouted through `td.Detail`),
  `internal/tui/detail.go` (left/right column split in `detailState`,
  `issueDetail`, `specDetail`), `internal/tui/tui.go` (two-column
  `viewDetail`, viewport sized to half-width, `detailColWidth`).
- Goldens regenerated: `detail-task`, `detail-spec`,
  `merge-confirm`, `reject-reason`, `status-pick`, `status-pick-moved`
  all show the new two-column layout (the detail view's frame is now
  half-width per side).
