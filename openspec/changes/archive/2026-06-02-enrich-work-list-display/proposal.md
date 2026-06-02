## Why

`td-254d01`: the work list renders every Issue the same way regardless of its
type (bug, feature, task, chore, epic) and ignores parent/child relationships,
which makes the TUI look like a flat dump rather than a real workplan. The user
also wants a blocked marker on tasks that are waiting on another. Surfacing
type, hierarchy and (later) blocked-by is what turns the list into a useful
working view.

## What Changes

- The work-list rows SHALL show a type indicator next to the ID so the user
  can scan bug vs feature vs chore vs epic vs task at a glance.
- A task with a parent SHALL be rendered indented under that parent, so an
  epic + its children read as a tree. Roots keep their current ordering;
  children follow their parent depth-first.
- The `Issue` model SHALL expose the task's `parent_id`. The board's refresh
  enriches each task with the `parent_id` td reveals only on `td show --json`.
- The blocked marker is **deferred** to a follow-up: `td list/show --json`
  doesn't currently surface `depends_on`, so we cannot detect blocked-by from
  data. Captured here as a future requirement and revisited when td exposes it.
- A `MockFixture()` adds a deterministic epic + two children + a blocked-by
  example to the replay engine so the goldens demonstrate the new layout.

## Capabilities

### New Capabilities

- (none — modifies two existing capabilities.)

### Modified Capabilities

- `view-work-list`: add type-indicator and hierarchy requirements; document
  the blocked-by placeholder.
- `02-board`: add a requirement that Issues expose the task's parent and that
  the board preserves parent/child ordering.

## Impact

- `internal/adapter/td/td.go` — `rawTask` gains `ParentID` (mapped from
  `parent_id`); new `Enrich` helper fetches per-task `td show --json` to fill
  fields td list strips out, called once per refresh by `board.List`.
- `internal/issue/issue.go` — `Task.ParentID string`; `Issue.Depth int`
  (rendering hint) and `Issue.Parent string` accessor.
- `internal/board/board.go` — `Assemble` (or a sibling helper) reorders Issues
  to depth-first by parent and stamps each Issue's depth.
- `internal/render/render.go` — `TaskTypeIcon(typ string) string` returns
  one of the canonical glyphs.
- `internal/tui/backlog.go` and `cmd/sindri/task.go` — render the type icon and
  indent rows by `iss.Depth`.
- `internal/tui/replay_fixtures.go` — `MockFixture()` returns a richer board
  showing the new layout (epic + children + types + a blocked placeholder).
- `internal/tui/testdata/frames/` — regenerated and new `list-mock` golden.
- No CLI flag changes.
