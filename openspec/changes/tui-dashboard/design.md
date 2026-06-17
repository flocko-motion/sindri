## Context

`internal/tui` today is a single read-mostly screen built directly on the hub
client's `BoardState`. We want a real dashboard: tabbed, full-height,
master-detail, navigable vi-style, and able to act (not just view). The design
keeps all derivation in the hub (the logic layer) so the TUI is a thin renderer —
consistent with the architecture (UIs are thin hub clients).

## Goals / Non-Goals

**Goals:**
- Full-terminal layout at any size; footer always the last two rows.
- Three tabs (Tasks/Agents/PRs) with live actionable counts.
- Tasks as a collapsible `parent_id` tree.
- Task↔PR cross-links in both directions.
- vi navigation; per-tab actions that drive the hub (new task, launch, tell,
  attach, merge).
- All counts/tree/joins computed hub-side; TUI only renders + dispatches.

**Non-Goals:**
- No new transport/endpoints — reuse `/state`, `/events`, `/log`, and the
  existing action calls.
- No editing of task fields beyond create (no in-TUI re-priority/re-parent yet).
- No multi-pane beyond list+detail; no mouse.

## Decisions

### D1 — Logic-layer section model

`hub.Sections` is an ordered `[]Section{Key, Title, Count func(BoardState) int}`.
The TUI iterates it to draw the tab strip (`[Count(state) Title]`) and to know
which tabs exist. Counts: tasks = count of **non-closed**, agents = running, PRs =
not-merged. Adding a section later is one list entry; the TUI changes nothing.

### D2 — Task tree in the logic layer

`hub.ArrangeTasks(tasks []store.Task, prs []store.PR) []TaskRow` returns the tasks
ordered as a tree — roots (sorted by priority then id), each followed by its
children recursively — with `Depth` set and `PR` set to a non-merged PR id for
that task (or ""). A task whose parent is absent from the set is treated as a
root so nothing is hidden. The TUI indents by `Depth`, shows `◆` when `PR != ""`,
and tracks collapsed node ids locally (fold state is a view concern).

### D3 — Master-detail, full-height layout

From `tea.WindowSizeMsg{w,h}`: tab strip = row 0; footer = last 2 rows; body =
`h-3` rows. Left selector column a fixed width (~34), a divider, detail pane =
remaining width. Both panes are padded/truncated to the body height so the frame
always fills the terminal. lipgloss for borders/joins.

### D4 — Navigation

- Tabs: `ctrl+h`/`ctrl+l` (prev/next, vi window style) and `1/2/3` (jump).
- Move selection: `j/k`, `g/G` (top/bottom), `ctrl+d/u` (half-page).
- Tasks tree fold: `h` collapse, `l` expand.
- Selection always drives the detail pane (no separate "open" step).
- `q`/`ctrl+c` quit.

### D5 — Actions (control surface) via the hub client

Footer row two lists the active tab's actions:
- Tasks: `n` new task (modal).
- Agents: `n` new (modal), `l` launch, `t` tell (modal), `a` attach.
- PRs: `m` merge.
Each calls the existing client method; `/events` then refreshes the view. `attach`
uses `tea.ExecProcess` to suspend the program and run `podman exec -it … tmux
attach`, resuming on detach. Modals use `bubbles/textinput`.

### D6 — Detail panes

- Tasks: id, title, type/priority/status, assignee (the agent whose `Task` ==
  this id), linked PR, labels, description/acceptance.
- Agents: identity + live state + the activity timeline (`client.Log`).
- PRs: PR metadata, **linked task** (id/title/status), then the diff
  (`client.PRInfo` → `PRDetail{PR, Task, Diff}`).

### D8 — One scrollable-pane primitive (pure, tested)

A pane is assigned a **fixed height**, and content of *any* length is displayed
inside it: shorter content is padded to fill, longer content scrolls. Every
overflowing region — the selector (list/tree) and the detail pane (diffs,
descriptions, timelines) — uses one primitive, so a pane is **always exactly its
assigned height** and no pane does its own offset math. This was the previous
TUI's recurring bug source (viewport arithmetic tangled into rendering/resize).

`internal/tui/scroll.Viewport` — a pure value type, no Bubble Tea coupling, so the
arithmetic is unit-tested on its own:

- State: `Height`, `Total`, `Offset`, optional `Cursor`.
- Ops (all clamped): `Up/Down`, `PageUp/PageDown`, `Top/Bottom`, `SetHeight(h)`
  (resize), `SetTotal(n)` (content changed).
- `Window() (start, end int)` — the visible line range.
- `Render(lines []string) string` — clips `lines` to the window **and pads to
  exactly `Height`**, so the caller always gets a fixed-height block whether
  content is shorter or longer than the pane.
- Two modes from the one type: **cursor-follow** (selector keeps `Cursor` in
  view) and **free-scroll** (detail pane, no cursor).

Unit tests cover the cases that historically broke: content shorter than the pane
(pad, no scroll), content longer (scroll), cursor at top/bottom edges, resize
shrinking below the cursor, content shrinking below the offset, and page near the
ends.

### D9 — Tasks filter toggle

The Tasks tab has a 3-way filter, cycled with `f` and shown in the footer:
**open → closed → all** (default open). "open" means *not done* — open +
in_progress + in_review (`!issue.Task.IsClosed()`); "closed" means the done
segment (closed/approved/merged). Because `BoardState.Tasks` carries all tasks
(D7), the filter is a pure client-side predicate over the tree — instant, no
fetch — and the tree/assignee/PR-mark logic operates on the filtered subset. The
tab badge always reflects non-closed regardless of the active filter.

### D7 — Data plumbing

- `store.Task` gains `ParentID`, `Description`, `Acceptance`; the `tasks` table
  gains the columns; `adapter/td/sqlite.go` selects `parent_id, description,
  acceptance` and `toStoreTask` maps them.
- `BoardState.Tasks` carries **all** tasks (every status), so the Tasks-tab
  filter (D9) can reach closed ones — sync with `issue.FilterAll`. The Tasks
  badge counts only the **non-closed** subset (the section count, D1); the tab
  defaults to the open filter.
- `PRInfo` fetches the linked task (`td.Get(pr.Task)`) into `PRDetail.Task` so it
  resolves even after the task closes on merge.

## Risks / Trade-offs

- **tea.ExecProcess + tmux attach** interaction (terminal handoff) — verify the
  TUI restores cleanly on detach. → prototype `attach` early.
- **Detail-pane wrapping/scrolling** for long diffs/descriptions — start with
  truncation to body height; add a scroll later if needed.
- **Tree fold state vs live updates** — when `/events` replaces the task set, map
  collapsed-ids by task id (not index) so folds survive refreshes.
- **Layout math at tiny sizes** — guard minimum widths/heights; degrade to a
  single column under a threshold.

## Open Questions

- Default fold state: expanded-all (simpler, see everything) vs collapse epics by
  default. Leaning expanded-all for v1.
- Should the PRs tab list merged PRs too (history) or only not-merged? Leaning
  show all, badge counts not-merged.
