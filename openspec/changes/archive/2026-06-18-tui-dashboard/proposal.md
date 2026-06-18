# Tabbed TUI dashboard

## Why

The current `internal/tui` is a skeleton: one scrolling screen listing agents,
PRs, and open tasks, read-mostly. It doesn't use the terminal well, has no task
hierarchy, no per-section overview, and only a couple of actions. It needs to be
a real control surface — the same things the CLI can do, but navigable.

## What Changes

- **Tabbed, full-height, master-detail.** A tab strip across the top, a
  left selector column, a detail pane on the right, and a fixed two-row footer
  pinned to the last two rows — the layout always fills the terminal regardless
  of item count.
- **Three sections, each a tab with a live count:** `[2 Tasks] [3 Agents] [1 PRs]`.
  The badge is the *actionable* count — non-closed tasks, **running** agents,
  **not-merged** PRs — and updates live over `/events`.
- **Sections are logic-layer data.** A `hub.Sections` list (key, title, count
  func over `BoardState`) is the single source of truth for which tabs exist and
  their badge counts; the TUI renders from it and never re-derives counts.
- **Tasks are a tree.** The Tasks selector renders the `parent_id` hierarchy
  (epic → feature → task), depth-indented, collapsible. Arrangement is a
  logic-layer function `hub.ArrangeTasks(tasks, prs)` returning depth-tagged rows
  annotated with a waiting-PR marker.
- **Cross-links both ways.** A task row marks `◆` when it has a non-merged PR; a
  PR's detail pane shows its linked task (id, title, status) above the diff.
- **vi navigation.** Tabs: `ctrl+h`/`ctrl+l` (+ `1/2/3`). Move: `j/k`, `g/G`,
  `ctrl+d/u`. Tree fold: `h`/`l`. Selection drives the detail pane live.
- **A control surface.** Per-tab actions via the hub client (shown in footer row
  two): Tasks `n` new; Agents `n` new · `l` launch · `t` tell · `a` attach; PRs
  `m` merge. `attach` suspends the TUI into the live tmux session and returns on
  detach. `n`/`t` use a small text-input modal.
- **Plumbing:** `store.Task` gains `ParentID` (+ `description`/`acceptance` for
  the detail pane); the `tasks` cache table + the direct SQLite reader carry
  them. `/state.Tasks` carries **all** tasks (synced with `FilterAll`) so the
  Tasks-tab filter can reach closed ones; the badge counts the non-closed subset.
- **Tasks filter.** The Tasks tab has a 3-way `f` toggle — **open → closed →
  all** (default open); "open" = not done (open/in_progress/in_review), "closed"
  = the done segment. A pure client-side predicate over the loaded tasks.

## Capabilities

### New Capabilities

- `view-tui`: the tabbed dashboard — layout (full-height master-detail, two-row
  footer), the section model with live counts, the task tree, the task↔PR
  cross-links, vi navigation, and the per-tab actions. It is a hub client and
  refuses to start without a running hub.

### Modified Capabilities

- `hub`: `BoardState` carries all non-closed tasks (not just open) and richer
  task fields (parent, description); the hub gains the section model
  (`Sections` + counts), task-tree arrangement (`ArrangeTasks`), and the linked
  task in `PRInfo` — the logic the TUI renders.

## Impact

- **New:** a rewritten `internal/tui` (tabbed master-detail Bubble Tea model),
  `bubbles/textinput` dependency for modals.
- **Changed:** `internal/hub` — `BoardState`/`AgentView`/task DTOs, `Sections`,
  `ArrangeTasks`, `PRInfo` (+ linked task); `internal/hub/store` (`Task.ParentID`,
  description; `tasks` table columns); `internal/adapter/td/sqlite.go` (read
  parent/description). Possibly `/state` task filter (all non-closed).
- **No new endpoints** — all of it rides existing `/state` + `/events` + `/log`.
