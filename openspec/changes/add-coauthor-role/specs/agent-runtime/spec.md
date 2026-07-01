# Agent runtime — delta

## MODIFIED Requirements

### Requirement: Agent sees only its own workspace

An agent SHALL see only its own git workspace and the single hub socket that is
its channel to the hub; the roster, the `.sindri/` directory, and other agents'
workspaces SHALL NOT be visible to it. For a worker or reviewer the workspace is
an isolated per-agent worktree named after the agent. The coauthor is the
deliberate exception: because it works the same material as the user, its
workspace IS the user's own working checkout (the repo root), so it is NOT
isolated from the user's tree — but even then the `.sindri/` directory SHALL
remain hidden from it (overlaid by an empty read-only directory), so it can
neither read nor corrupt hub state in the shared checkout.

#### Scenario: Isolated agent sees only its own worktree

- **WHEN** a worker or reviewer inspects its environment
- **THEN** it sees only its own per-agent worktree and its hub socket — not the
  roster, `.sindri/`, or any other agent's workspace

#### Scenario: Coauthor shares the user's checkout but not .sindri

- **WHEN** a coauthor inspects its environment
- **THEN** its `/workspace` is the user's actual repository checkout (edits are
  shared with the user), yet `.sindri/` is still hidden, so it cannot read or
  write hub state

## ADDED Requirements

### Requirement: The coauthor is a fourth role driven directly by the user

Sindri SHALL support a fourth agent role, the coauthor, alongside worker,
reviewer, and planner. A coauthor SHALL run with the same runtime shape as any
agent — interactive Claude in a named tmux session, driven by the same thin
browser client — but it SHALL NOT be assigned backlog tasks and SHALL NOT be
driven by the managed act-report-idle loop. It works directly with the user in
the shared checkout: the user steers it through its terminal (provenance-stamped
messages), and it edits files and uses git itself. Its briefing SHALL tell it it
is the coauthor and that there is no task queue and no review gate.

#### Scenario: Coauthor registered as a distinct role

- **WHEN** an agent is registered with the coauthor role
- **THEN** it is accepted as a valid role distinct from worker, reviewer, and
  planner, and launches with the same tmux/browser runtime as any agent

#### Scenario: Coauthor is never handed a task

- **WHEN** open backlog tasks exist and a coauthor asks the hub for its next action
- **THEN** the hub does not assign it a task; only workers claim backlog tasks

### Requirement: Coauthor next-action is freestyle and never blocks

A coauthor SHALL NOT block waiting for assigned work. Its next-action directive
SHALL be a freestyle brief — work with the user in the shared checkout, editing
files and using git directly — returned immediately rather than blocking on a
work queue. The user drives the coauthor by injecting messages into its session;
when the user is quiet the coauthor SHALL wait rather than invent work.

#### Scenario: Next action returns immediately

- **WHEN** a coauthor asks the hub for its next action
- **THEN** the hub returns the freestyle brief at once, without blocking on a queue
  or assigning a task

#### Scenario: Driven by the user, not the loop

- **WHEN** the user types an instruction into the coauthor's session
- **THEN** the coauthor acts on it directly, committing with git itself rather than
  through a hub submit verb
