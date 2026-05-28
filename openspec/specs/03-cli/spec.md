# CLI

## Purpose

Defines the host CLI (`sindri`): the human planning and review interface. Like
the TUI it is a thin renderer over the shared work-item state, and it is the
interface where a human approves and merges. It presents the same items, with
the same fields, as the TUI — the two are interchangeable.

## Requirements

### Requirement: Renders shared state

CLI commands that list or show work items SHALL obtain them from `board.List` —
the same refresh path the TUI uses — so both interfaces show the same data.

#### Scenario: Listing tasks

- **WHEN** `sindri task list` runs
- **THEN** it renders `board.List` issues, identical in content to the TUI backlog

### Requirement: Field parity with the TUI

For any given item, a CLI detail command SHALL show the same fields the TUI
detail view shows for that item. Styling SHALL come from the rendering module.

#### Scenario: Task detail parity

- **WHEN** `sindri task view <id>` and the TUI detail both show one task
- **THEN** they present the same fields (status, gates, PRs, comments, ...)

### Requirement: Work list

`sindri task list` SHALL show issues as rows — spec-only items, tasks, and
spec-linked tasks — with id, priority, updated time, status, title, PR
sub-rows, and review-gate rows. It SHALL hide closed items by default, with
`--all`, `--open`, and `--closed` to adjust.

#### Scenario: Default hides closed

- **WHEN** `sindri task list` runs without flags
- **THEN** closed/approved/merged tasks are omitted

### Requirement: PR review flow

`sindri pr` SHALL provide the review flow: select the next reviewable PR
(`next`), inspect it (`info`/`view`), build it in a scratch worktree (`try`),
mark review gates (`review`), and act on it. The selected PR SHALL persist so
later subcommands default to it.

#### Scenario: Selecting and inspecting

- **WHEN** `sindri pr next` selects a PR and `sindri pr info` is run with no id
- **THEN** info reports the selected PR plus its task summary and gates

### Requirement: Human-only approve and merge

`sindri pr approve` and `sindri pr merge` SHALL require explicit human
confirmation before acting, since agents must not approve or merge their own
work. Merging SHALL enforce review gates and close the task.

#### Scenario: Confirmation prompt

- **WHEN** approve or merge is invoked
- **THEN** the command prompts to confirm a human is acting before proceeding

### Requirement: Mutations through adapters

CLI commands SHALL perform task and PR mutations through the logic layer (the
td adapter and the PR store), never by invoking external tools directly.

#### Scenario: Creating a task

- **WHEN** `sindri task new` runs
- **THEN** it creates the task via the td adapter, not a direct CLI call

## Structure

`cmd/sindri/` — all files `type: command`/`entrypoint`, thin over the logic
layers:

- `main.go` — entrypoint; wires the command tree.
- `task.go` — `task list/new/view/comment` (the work list and task detail).
- `pr.go` — the `pr` review flow (list/info/view/next/try/approve/merge/reject/
  review).
- `tui.go` — launches the TUI. `review.go` — launches the reviewer worker.
- `lint.go` — wires the linters.

Worker management (`sindri worker ...`) mirrors the TUI workers view and is
served by `internal/worker`.
