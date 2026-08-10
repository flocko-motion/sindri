# Agent Runtime — delta

## ADDED Requirements

### Requirement: Context size is read from the session transcript

A coding-agent backend SHALL report the current context size of a live session,
read from wherever that backend persists its own transcript on disk — not from
pane text. For Claude Code, this is the last assistant message's
`input_tokens + cache_read_input_tokens + cache_creation_input_tokens` in the
newest transcript file under the agent's home. An agent with no recorded usage
yet SHALL report that no measurement exists, distinct from a measurement of
zero.

#### Scenario: A live session's context size is read off disk

- **WHEN** the board or the assignment gate asks for an agent's context size
- **THEN** the figure comes from the session transcript's own recorded usage,
  not from pattern-matching the tmux pane

#### Scenario: A freshly launched agent has no measurement

- **WHEN** an agent has not yet replied once in its current session
- **THEN** its context size is reported as unmeasured, not as zero tokens used

### Requirement: A human can clear a full worker's session at a leaf boundary

Sindri SHALL provide a way to send Claude Code's own `/clear` into an agent's
live session and re-serve its current directive afterward, so the agent resumes
exactly as it would after a fresh launch. This SHALL be refused while the agent
holds a task or a container: a held task's memory of the worktree's contents
would go stale silently, since only the task's own next claim resets the
worktree. Invoking the action IS the confirmation; no further prompt is
required.

#### Scenario: Clearing a full, idle worker

- **WHEN** a human clears an agent that holds no task and no container
- **THEN** `/clear` is sent into its session and its directive is re-served,
  landing it back at "ask for work"

#### Scenario: Clearing a worker mid-task is refused

- **WHEN** a human attempts to clear an agent that currently holds a task or a
  container
- **THEN** the action is refused, explaining that clearing only applies at a
  leaf task boundary
