# view-tui — delta

## ADDED Requirements

### Requirement: Yanking copies what the focus is on

The yank key SHALL copy according to where the focus is. With the detail pane focused it SHALL
copy the focused item's own value, so a single field can be lifted out alone. With the list
focused it SHALL copy the selected row's id, except on the PRs tab.

From the PRs list the yank SHALL copy a block identifying the PR: its id, its status, its author,
its task's id and title, its branch, and its author's worktree path where that path resolves. The
block SHALL NOT include the diff, which the full detail view carries.

The identifying block and the full detail view SHALL be derived from one definition, so that a
field added to either appears in both.

Where the PR's detail has not yet been fetched for the selected row, the yank SHALL copy the id
rather than a block built from another PR's detail.

#### Scenario: A PR is yanked from the list

- **WHEN** the user yanks a selected PR from the PRs list
- **THEN** the clipboard holds its id, status, author, task id and title, branch, and its author's
  worktree path, and no diff

#### Scenario: A single field is yanked from the detail

- **WHEN** the detail pane is focused on an item and the user yanks
- **THEN** the clipboard holds that item's value alone

#### Scenario: The author's worktree is gone

- **WHEN** a PR's author no longer has a workspace
- **THEN** the yanked block omits the path and still carries the rest

#### Scenario: The detail has not arrived yet

- **WHEN** the user yanks a PR whose detail is still being fetched
- **THEN** the clipboard holds that PR's id, never another PR's fields
