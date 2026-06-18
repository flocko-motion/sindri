# View: TUI dashboard

## Purpose

Defines the TUI dashboard — a full-terminal, tabbed master-detail control surface
for sindri. The dashboard is a pure hub client: it gets all data from the hub and
performs all mutations through it, holding no domain logic of its own. This
capability covers its layout, its section-driven tabs with live actionable counts,
the collapsible task tree, task/PR cross-linking, fixed-height scrollable panes,
vi navigation, the tasks filter toggle, the per-tab action surface, and its
dependence on a running hub.

## Requirements

### Requirement: Full-terminal tabbed master-detail layout

The TUI SHALL fill the whole terminal at any size: a tab strip on the top row, a
left selector column and a right detail pane below it, and a footer pinned to the
last two rows. The layout SHALL NOT collapse or leave dead space when there are
few items — the panes are sized from the terminal height so the footer is always
on the last row.

#### Scenario: Few items still fills the frame

- **WHEN** a tab has only one or two items
- **THEN** the selector/detail panes still extend to full height and the footer
  remains on the last two rows

#### Scenario: Resize

- **WHEN** the terminal is resized
- **THEN** the panes and footer re-flow to the new size, footer still last

### Requirement: Tabs are sections with live actionable counts

The TUI SHALL present tabs from the hub's section model, each titled `[<n> <Title>]`
where `<n>` is the section's actionable count: non-closed tasks, running agents,
and not-merged PRs. The counts SHALL update live as board state changes. The TUI
SHALL NOT compute the counts itself — it renders the hub-provided count per
section.

#### Scenario: Count reflects state

- **WHEN** an agent starts running or a PR is merged
- **THEN** the corresponding tab badge updates without a manual refresh

### Requirement: Tasks are shown as a collapsible tree

The Tasks tab selector SHALL render tasks as a tree by their parent hierarchy,
depth-indented, with parents before children. Nodes with children SHALL be
collapsible. A task whose parent is not in the visible set SHALL render as a root
so it is never hidden. The tree arrangement SHALL come from the hub (the logic
layer), not be derived in the TUI.

#### Scenario: Hierarchy displayed

- **WHEN** tasks have parent/child relationships
- **THEN** children appear indented under their parent, deepest last

#### Scenario: Collapse and expand

- **WHEN** the user collapses a parent node
- **THEN** its descendants are hidden until expanded, and the fold survives a
  live state refresh

### Requirement: Task and PR are cross-linked in both views

A task row SHALL be marked when it has a non-merged PR. A PR's detail SHALL show
its linked task — at least the task id, title, and status — alongside the diff.

#### Scenario: Task marks a waiting PR

- **WHEN** a task has a non-merged PR
- **THEN** its row shows a PR marker

#### Scenario: PR shows its task

- **WHEN** a PR is selected
- **THEN** the detail pane shows the linked task's id, title, and status, and the
  diff

### Requirement: Panes are fixed-height scrollable regions

Every content region — the selector and the detail pane — SHALL be a fixed-height
pane that displays content of any length: content shorter than the pane is padded
to fill it, content longer than the pane scrolls. A pane SHALL always render
exactly its assigned height. All such regions SHALL use one shared scroll
primitive rather than per-pane offset logic.

#### Scenario: Content shorter than the pane

- **WHEN** a pane's content is shorter than its height
- **THEN** it is padded to full height (no scrolling), and the layout around it is
  unaffected

#### Scenario: Content longer than the pane

- **WHEN** a pane's content exceeds its height
- **THEN** it scrolls within its fixed height, and the selected/focused line stays
  in view

### Requirement: vi navigation

The TUI SHALL navigate vi-style: `ctrl+h`/`ctrl+l` switch tabs (and `1`/`2`/`3`
jump to one); `j`/`k` move the selection, `g`/`G` jump to top/bottom; in the task
tree `h`/`l` collapse/expand. Moving the selection SHALL update the detail pane
immediately (no separate open step).

#### Scenario: Tab switch

- **WHEN** the user presses `ctrl+l`
- **THEN** the next tab becomes active

#### Scenario: Selection drives detail

- **WHEN** the user moves the selection with `j`/`k`
- **THEN** the detail pane shows the newly selected item

### Requirement: Tasks filter toggle

The Tasks tab SHALL provide a filter, cycled with `f`, over three states: open →
closed → all, defaulting to open. "open" SHALL mean not-done (open, in_progress,
in_review); "closed" SHALL mean the done segment (closed/approved/merged); "all"
SHALL mean both. The active filter SHALL be shown in the footer and applied to
the displayed task tree. The tab's badge count SHALL remain the non-closed count
regardless of the active filter.

#### Scenario: Toggle to closed

- **WHEN** the user presses `f` until the filter is "closed"
- **THEN** the tree shows only done tasks, while the tab badge still counts
  non-closed tasks

#### Scenario: Default is open

- **WHEN** the Tasks tab is first shown
- **THEN** it lists not-done tasks (open, in_progress, in_review)

### Requirement: The dashboard is a control surface

Each tab SHALL offer its actions (shown in the footer's second row), performed
via the hub: Tasks — create a task; Agents — new, launch, tell, attach; PRs —
merge. Attaching SHALL hand the terminal to the agent's live tmux session and
return to the TUI on detach. After an action, the view SHALL reflect the change
(live, via board events).

#### Scenario: Merge from the PRs tab

- **WHEN** the user merges the selected approved PR
- **THEN** the hub merges it and the board updates to show it merged

#### Scenario: Attach and return

- **WHEN** the user attaches to an agent
- **THEN** the TUI suspends into the agent's live terminal and resumes when the
  user detaches

### Requirement: The TUI is a hub client

The TUI SHALL get all data from the hub (`/state` + `/events`) and perform all
mutations through it, holding no domain logic of its own. It SHALL refuse to
start when no hub is running.

#### Scenario: No hub

- **WHEN** the TUI starts and no hub is running
- **THEN** it exits with a message to start the hub
