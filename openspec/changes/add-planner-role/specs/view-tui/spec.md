# View: TUI dashboard — delta

## MODIFIED Requirements

### Requirement: The dashboard is a control surface

Each tab SHALL offer its actions (shown in the footer's second row), performed
via the hub: Tasks — create a task, and approve or reject a planner-proposed task
that is still under the approval gate; Agents — new (worker/reviewer/planner),
launch, tell, attach; PRs — merge. Attaching SHALL hand the terminal to the
agent's live tmux session and return to the TUI on detach. After an action, the
view SHALL reflect the change (live, via board events).

#### Scenario: Merge from the PRs tab

- **WHEN** the user merges the selected approved PR
- **THEN** the hub merges it and the board updates to show it merged

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

## ADDED Requirements

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
