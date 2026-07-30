# Workers — delta

## ADDED Requirements

### Requirement: No direct task-tracker access in the pod

The agent pod image SHALL NOT include the task-tracker CLI (`td`). An agent SHALL
reach task state — reads and writes alike — only through the hub over its socket;
the hub is the single writer of task state, operating on the main checkout. This
keeps the task tracker's runtime files (`.todos/`) from ever being written in, and
committed from, an agent's worktree, so a PR branch carries only the agent's own
changes.

#### Scenario: No td in the pod

- **WHEN** an agent pod starts
- **THEN** no `td` binary is present, and the agent can read or change task state
  only via the hub over its socket

#### Scenario: Branch stays free of task-tracker churn

- **WHEN** an agent's work is committed
- **THEN** the commit contains only the agent's changes and never `.todos/`
  runtime churn, because nothing in the worktree can write the tracker
