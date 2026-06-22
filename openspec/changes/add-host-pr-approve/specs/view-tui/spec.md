# view-tui — delta

## MODIFIED Requirements

### Requirement: The dashboard is a control surface

Each tab SHALL offer its actions (shown in the footer's second row), performed
via the hub: Tasks — create a task; Agents — new, launch, tell, attach; PRs —
approve, reject, and merge. The PRs approve action SHALL be the human approve
(distinct from requesting an agentic review). Attaching SHALL hand the terminal to
the agent's live tmux session and return to the TUI on detach. After an action,
the view SHALL reflect the change (live, via board events). For an action that is
not instantaneous — notably merge — the view SHALL give immediate feedback the
moment it is invoked (e.g. a transient "merging" status on the row) rather than
appearing to hang until the hub's board event lands.

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
