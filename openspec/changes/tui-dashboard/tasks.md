# Tasks

## 1. Logic-layer plumbing (hub + store + adapter)

- [ ] 1.1 `store.Task` gains `ParentID`, `Description`, `Acceptance`; add the
      columns to the `tasks` table; `ReplaceTasks`/`UpsertTask`/`scanTasks` carry them
- [ ] 1.2 `adapter/td/sqlite.go`: select `parent_id, description, acceptance`;
      `toStoreTask`/`scanDBTask` map them
- [ ] 1.3 Board carries all non-closed tasks (open + in_progress + in_review), each
      with parent + description; add `store.NonClosedTasks()` or adjust assembly
- [ ] 1.4 Confirm a task resolves its working agent (assignee) from the board

## 2. Section model + tree + cross-links (hub)

- [ ] 2.1 `hub.Section{Key,Title,Count func(BoardState) int}` + ordered `Sections`
      (tasks=non-closed, agents=running, prs=not-merged)
- [ ] 2.2 `hub.TaskRow{store.Task; Depth int; PR string}` +
      `ArrangeTasks(tasks, prs) []TaskRow` (tree by parent, priority-sorted roots,
      absent-parent→root, PR-annotated)
- [ ] 2.3 `PRInfo` → `PRDetail{PR, Task store.Task, Diff}` (fetch task via td.Get)
- [ ] 2.4 Unit tests: section counts, ArrangeTasks ordering/depth/PR-mark/orphan-parent

## 3. Scroll primitive (build + test first — everything renders through it)

- [ ] 3.1 `internal/tui/scroll.Viewport` — pure value type (`Height`, `Total`,
      `Offset`, optional `Cursor`); clamped `Up/Down/PageUp/PageDown/Top/Bottom`,
      `SetHeight`, `SetTotal`, `Window()`, and `Render(lines) string` that clips to
      the window and pads to exactly `Height`. Cursor-follow + free-scroll modes
- [ ] 3.2 Exhaustive unit tests: content shorter than pane (pad), longer (scroll),
      cursor at edges, resize below cursor, content shrink below offset, page near
      ends, total<height

## 4. TUI rewrite (internal/tui)

- [ ] 4.1 Full-height master-detail frame from `WindowSizeMsg`: tab strip (row 0),
      body (h-3), footer (last 2 rows); left selector + divider + detail pane,
      each a fixed-height `scroll.Viewport`
- [ ] 4.2 Tab strip from `hub.Sections` with `[<n> <Title>]` live counts; active
      tab highlighted
- [ ] 4.3 vi nav: `ctrl+h/l`+`1/2/3` tabs; `j/k`,`g/G`,`ctrl+d/u` move (drives the
      selector viewport's cursor); selection drives the detail pane
- [ ] 4.4 Tasks tab: tree render (indent by Depth, `◆` PR marker), `h/l`
      collapse/expand with fold state keyed by task id (survives refresh)
- [ ] 4.5 Detail panes (each a free-scroll viewport): task (fields + assignee + PR
      + description), agent (state + `client.Log` timeline), pr (`PRInfo`: meta +
      linked task + diff)
- [ ] 4.6 Two-row footer: row1 global nav, row2 context actions for the active tab
- [ ] 4.7 Actions via the hub client: Tasks `n` (new-task modal); Agents `n`
      (modal),`l` launch,`t` tell (modal),`a` attach; PRs `m` merge
- [ ] 4.8 `a` attach via `tea.ExecProcess` (podman exec -it … tmux attach), resume
      on detach
- [ ] 4.9 `bubbles/textinput` modal for `n`/`t`
- [ ] 4.10 Live updates: `Watch` channel → re-render; `r` forces task re-sync;
      refuse to start without a hub (keep the guard + test)

## 5. Validation

- [ ] 5.1 `openspec validate tui-dashboard --strict` passes
- [ ] 5.2 `go test ./...` + `sindri lint all` green; no import cycles
- [ ] 5.3 Manual: launch agents + tasks, drive every tab/action; verify full-height
      layout, counts, tree fold, cross-links, scroll (short+long content), attach
      round-trip
