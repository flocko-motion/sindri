# Action: Comment

## Purpose

Defines adding a comment to a task, independent of any interface (`sindri task
comment`, the TUI detail comment input, and the agent's `gh issue comment` all
perform this one action).

## Requirements

### Requirement: Add a comment

Adding a comment SHALL append the given text to the task's comment thread
through the td adapter. An empty comment SHALL be rejected.

#### Scenario: Comment added

- **WHEN** non-empty text is submitted for a task
- **THEN** it is appended to that task's comments via the td adapter

#### Scenario: Empty comment

- **WHEN** an empty comment is submitted
- **THEN** the action is refused
