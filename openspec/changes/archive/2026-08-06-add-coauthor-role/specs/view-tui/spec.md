# View: TUI dashboard — delta

## MODIFIED Requirements

### Requirement: The dashboard is a control surface

Each tab SHALL offer its actions (shown in the footer's second row), performed
via the hub: Tasks — create a task; Agents — new (worker / reviewer / planner /
coauthor), launch, tell, attach; PRs — merge. Attaching SHALL hand the terminal
to the agent's live tmux session and return to the TUI on detach. After an
action, the view SHALL reflect the change (live, via board events).

#### Scenario: Merge from the PRs tab

- **WHEN** the user merges the selected approved PR
- **THEN** the hub merges it and the board updates to show it merged

#### Scenario: Attach and return

- **WHEN** the user attaches to an agent
- **THEN** the TUI suspends into the agent's live terminal and resumes when the
  user detaches

#### Scenario: New-agent picker offers the coauthor role

- **WHEN** the user creates a new agent from the Agents tab
- **THEN** the role picker offers coauthor alongside worker, reviewer, and planner
