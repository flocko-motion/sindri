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
where `<n>` is the section's actionable count. The Agents and PRs tabs SHALL be
global across all projects — each row identifying its repo — so counts reflect
running agents and not-merged PRs everywhere. The Tasks tab SHALL be scoped to the
currently selected repo. The counts SHALL update live as board state changes; the
TUI SHALL NOT compute them itself.

#### Scenario: Count reflects state across repos

- **WHEN** an agent starts running or a PR is merged in any repo
- **THEN** the corresponding global tab badge updates without a manual refresh

#### Scenario: Tasks follow the selected repo

- **WHEN** the user switches the selected repo
- **THEN** the Tasks tab shows that repo's tasks, while the Agents and PRs tabs
  stay global

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
via the hub: Tasks — create a task, and approve or reject a planner-proposed task
that is still under the approval gate; Agents — new (worker/reviewer/planner),
launch, tell, attach; PRs — approve, reject, and merge. The PRs approve action
SHALL be the human approve (distinct from requesting an agentic review). Attaching
SHALL hand the terminal to the agent's live tmux session and return to the TUI on
detach. After an action, the view SHALL reflect the change (live, via board
events). For an action that is not instantaneous — notably merge — the view SHALL
give immediate feedback the moment it is invoked (e.g. a transient "merging" status
on the row) rather than appearing to hang until the hub's board event lands.

#### Scenario: Approve from the PRs tab

- **WHEN** the user approves the selected open PR
- **THEN** the hub marks it approved and the row updates to show it approved,
  ready to merge

#### Scenario: Merge from the PRs tab

- **WHEN** the user merges the selected approved PR
- **THEN** the hub merges it and the board updates to show it merged

#### Scenario: Immediate merge feedback

- **WHEN** the user triggers a merge on a PR
- **THEN** the row immediately shows a transient "merging" indicator, replaced by
  "merged" when the hub confirms the merge (or cleared if the merge fails)

#### Scenario: Attach and return

- **WHEN** the user attaches to an agent
- **THEN** the TUI suspends into the agent's live terminal and resumes when the
  user detaches

#### Scenario: Approve a planner proposal

- **WHEN** the user approves a gated planner-proposed task on the Tasks tab
- **THEN** the hub clears its approval gate, the task becomes claimable, and the
  view reflects the change

#### Scenario: Reject a planner proposal

- **WHEN** the user rejects a gated planner-proposed task with a comment
- **THEN** the hub records the rejection, the comment is delivered to the planner,
  and the task stays hidden from workers
#### Scenario: New-agent picker offers the coauthor role

- **WHEN** the user creates a new agent from the Agents tab
- **THEN** the role picker offers coauthor alongside worker, reviewer, and planner

### Requirement: The TUI is a hub client

The TUI SHALL get all data from the single global hub (`/state` + `/events`) and
perform all mutations through it, holding no domain logic of its own. When no hub is
running it SHALL auto-start a background hub rather than refusing.

It SHALL reach the hub only through the client and the exchange format, importing no
other part of the core — no hub package, no persistence package, no adapter the hub
owns. Where it needs something the hub knows, it SHALL ask the hub rather than
computing it in its own process: whether an optional external tool is installed is
the hub's answer to give, because the hub is the process that runs it.

#### Scenario: No hub yet

- **WHEN** the TUI starts and no hub is running
- **THEN** it starts a background hub, then connects

#### Scenario: The TUI imports no core package

- **WHEN** the TUI package is compiled
- **THEN** it imports the client and the exchange format, and no hub or persistence
  package

#### Scenario: Tool availability is the hub's answer

- **WHEN** the TUI reports that an optional external tool is missing
- **THEN** the finding came from the hub, which is the process that would invoke the
  tool, rather than from the TUI probing its own environment

### Requirement: Repo switcher scopes the per-repo view

The TUI SHALL make the active repo a first-class, always-visible part of the
interface, and SHALL provide a switcher to change it.

The **active repo name SHALL be persistently visible** in the top bar (not only
inside an overlay), rendered in that repo's deterministic color scheme, so the user
can tell at a glance which repo the Tasks tab and any `repo`-scoped view reflect.

The **switcher SHALL be a picker overlay, not a tab strip** — the number of repos
may be large, so a fixed tab row would not scale. The overlay SHALL list the
registered projects (from the project registry) ordered most-relevant first: repos
with **live agents** on top, then by **recency** (last used), then the rest; and it
SHALL offer a typeahead filter to narrow a long list. Selecting a repo SHALL scope
the Tasks tab (and any other per-repo view) to it.

#### Scenario: Active repo always visible

- **WHEN** the TUI is showing any tab
- **THEN** the current repo's name is visible in the top bar, in that repo's color

#### Scenario: Switching repos

- **WHEN** the user opens the switcher overlay and picks a repo
- **THEN** the per-repo view rescopes to it and the top-bar indicator updates

#### Scenario: Switcher ordering and scale

- **WHEN** the switcher overlay is opened with many repos registered
- **THEN** repos with live agents appear first, then by recency, and a typeahead
  filter is available — it is a scrollable list, never a fixed tab row

#### Scenario: Rows carry their repo

- **WHEN** the Agents or PRs tab lists entries from more than one repo
- **THEN** each row shows which repo it belongs to

### Requirement: Each project has a deterministic color scheme

The TUI SHALL give each project a color scheme derived deterministically from its
stable key (`repoTag`), so the same repo always renders in the same colors across
sessions. A scheme SHALL be a *(primary, accent)* pair selected from a fixed palette
by hashing the project key, giving a space of `primary × accent` combinations large
enough that the handful of repos in use rarely collide. The current project's scheme
SHALL tint the board chrome (the active-repo indicator / header), and per-row repo
tags SHALL carry their project's color, so entries from different repos are visually
separable.

#### Scenario: Stable color per repo

- **WHEN** the same repo is shown in different sessions
- **THEN** it renders with the same color scheme both times

#### Scenario: Repos are visually distinguishable

- **WHEN** the Agents or PRs tab shows rows from several repos
- **THEN** each repo's rows carry its own scheme's color, and the selected repo tints
  the board chrome

### Requirement: Agents and PRs tabs have a global/repo scope toggle

The Agents and PRs tabs SHALL each offer a scope toggle between `global` and `repo`,
defaulting to `global`. In `global` the tab SHALL show the whole fleet across every
registered repo, each row repo-tagged. In `repo` the tab SHALL show only the active
repo's entries (the switcher's selection). The active scope SHALL be shown in the
footer. This is a view filter only; it SHALL NOT change what data the hub holds, and
the Tasks tab SHALL remain always scoped to the active repo.

#### Scenario: Default is global

- **WHEN** the Agents or PRs tab is first shown
- **THEN** it lists entries across all repos, each tagged with its repo

#### Scenario: Narrow to the active repo

- **WHEN** the user toggles the Agents tab scope to `repo`
- **THEN** it shows only the active repo's agents, and the footer reflects `repo`
  scope

### Requirement: Repo configuration is editable in the TUI

The TUI SHALL let a user edit a repo's `.sindri/config.yaml` through a form over its
keys (`architecture`, `containerfile`, `review_prompt`, `github.issues`), performed
through the hub. An invalid entry SHALL be reported to the user and SHALL NOT be
persisted as a broken config; hand-editing the YAML file directly SHALL remain
equally valid.

#### Scenario: Edit config via a form

- **WHEN** the user opens the repo config form, changes a value, and saves
- **THEN** the hub writes `.sindri/config.yaml` and the change takes effect on the
  next load

#### Scenario: Invalid config rejected

- **WHEN** the user enters a value that fails config validation (e.g. a path that
  escapes the repo)
- **THEN** the form reports the error and does not persist a broken config

### Requirement: Planner proposals are marked under the approval gate

A task proposed by a planner and still awaiting the user's decision SHALL be
visually distinguished in the Tasks tab from a normal backlog task, and its detail
SHALL show its approval state (pending, approved, or rejected) and any rejection
comment. Only a task under the gate (pending or rejected) SHALL be a valid target
for the approve/reject actions.

#### Scenario: Gated proposal is marked

- **WHEN** a planner-proposed task is pending or rejected
- **THEN** its row is distinguished from a normal task and its detail shows the
  approval state and any comment

#### Scenario: Approve/reject only on gated tasks

- **WHEN** the selected task has no unresolved approval (a normal task)
- **THEN** the approve/reject actions do not apply to it

