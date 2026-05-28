# TUI

## Purpose

Defines the terminal UI (`sindri tui`): a thin Bubble Tea renderer over the
shared work-item state. It presents the same data as the CLI and performs all
mutations through the logic layer, never reimplementing domain rules.

## Requirements

### Requirement: Renders shared state

The TUI SHALL obtain its work items from `board.List` — the same single refresh
path the CLI uses — so the two interfaces always show the same data.

#### Scenario: Refresh

- **WHEN** the TUI refreshes (on a tick, after an action, or on manual refresh)
- **THEN** it re-runs `board.List` and renders the returned `[]issue.Issue`

### Requirement: Backlog view

The TUI SHALL provide a backlog view listing issues as flat rows — spec-only
items, tasks, and tasks linked to specs — each row showing id, priority,
updated time, status, and title, with PR sub-rows and review-gate rows beneath
their task. Colors SHALL come from the rendering module.

#### Scenario: Worker on a task

- **WHEN** a worker is assigned to a task
- **THEN** that row's status shows the worker (🔨 name) instead of the raw status

#### Scenario: Orphaned in_progress

- **WHEN** a task is in_progress with no worker
- **THEN** its status is shown as a warning, distinct from a worked task

### Requirement: Workers view

The TUI SHALL provide a workers view listing each worker with status, current
task, branch, and workspace path. The active view (backlog vs workers) SHALL be
selectable by hotkey.

#### Scenario: Toggling views

- **WHEN** the view-toggle hotkey is pressed
- **THEN** the left column switches between the backlog and workers views

### Requirement: Detail drill-down

Pressing enter on a row SHALL open a full detail view of the item (metadata,
description, review gates, worker, PRs, comments). The detail SHALL refresh in
place when the underlying state changes.

#### Scenario: Open detail

- **WHEN** the user presses enter on a backlog row
- **THEN** a detail view for that issue or PR opens, scrollable with j/k

### Requirement: Actions go through the logic layer

Detail-view actions — comment, change status, approve, merge, reject — SHALL
call the logic layer (the td adapter and the PR store), never reimplementing
the operation. Merge and reject SHALL require an explicit confirmation step.

#### Scenario: Destructive action

- **WHEN** the user triggers merge or reject
- **THEN** a confirmation is required before the action runs

#### Scenario: Comment

- **WHEN** the user adds a comment in the detail view
- **THEN** it is written through the td adapter, not a direct CLI call

## Structure

`internal/tui/` — all files `type: ui`, thin over `board`/`issue`/`render`:

- `tui.go` — root Bubble Tea model: views, navigation, refresh, detail.
- `actions.go` — detail actions (comment/status/approve/merge/reject) and the
  confirmation/comment-input handling.
- `backlog.go` — builds and renders the backlog rows from `[]issue.Issue`.
- `detail.go` — multi-pane detail rendering for issue/PR/worker.
- `workers.go` — workers-view rendering.
- `create_task.go` — the new-task modal (creates via the td adapter).
- `keys.go` — keybindings. `styles.go` — chrome styles. `notify.go` — flash bar.
- `data.go` — refresh plumbing (board.List + worker.List; detail text via td).
