# Workers — delta

## ADDED Requirements

### Requirement: An agent may hold a container plus a rolling current subtask

In the container workflow an agent's assignment SHALL be a container task together
with a *current subtask* drawn from the container's open children. The current
subtask rolls forward as each one is checkpointed; the agent SHALL NOT block
between subtasks. The one deliberate pause is a milestone PR, which parks the
agent until the human merges, after which it resumes the same container. The hub
owns both the container assignment and the current subtask as durable state
(recoverable after a restart, like all worker-to-task mapping). When the container
closes, the agent is freed and rejoins normal (leaf) assignment.

#### Scenario: Working state spans subtasks

- **WHEN** an agent finishes one subtask of its container and takes the next
- **THEN** it stays in a working state throughout, never parking in a blocked
  "submitted" state between subtasks

#### Scenario: Recovered after restart

- **WHEN** the hub restarts while an agent holds a container
- **THEN** the container assignment and the current subtask are reloaded from
  durable state, not guessed from branch or worktree position

#### Scenario: Freed on container completion

- **WHEN** an agent's container is closed
- **THEN** the agent is freed and becomes eligible for normal leaf assignment again
