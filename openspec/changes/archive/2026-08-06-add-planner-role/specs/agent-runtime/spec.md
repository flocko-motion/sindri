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
neither read nor corrupt hub state in the shared checkout. The hub MAY tell an
agent its own role — it briefs the agent in a role-specific way and the `status`
verb reports the role — but an agent SHALL NOT be able to enumerate the roster,
address or observe another agent, or read `.sindri/`.

#### Scenario: Agent knows its own role but not the roster

- **WHEN** an agent inspects its environment
- **THEN** it knows its own role from the hub's briefing, but it cannot enumerate
  the roster, see other agents' workspaces, or read `.sindri/`

#### Scenario: No cross-agent visibility

- **WHEN** an agent tries to discover or address another agent
- **THEN** it cannot — only the hub holds the roster and the routing tables


#### Scenario: Isolated agent sees only its own worktree

- **WHEN** a worker or reviewer inspects its filesystem
- **THEN** it finds its own worktree and its hub socket, and neither the roster,
  `.sindri/`, nor another agent's workspace

#### Scenario: Coauthor shares the user's checkout but not .sindri

- **WHEN** a coauthor inspects its filesystem
- **THEN** its `/workspace` is the user's actual repository checkout (edits are
  shared with the user), yet `.sindri/` is still hidden, so it cannot read or
  write hub state

#### Scenario: Role invisible to the agent

- **WHEN** an agent inspects its environment
- **THEN** it cannot determine whether it is a worker or a reviewer; only the hub
  knows the role

#### Scenario: Other projects invisible

- **WHEN** an agent tries to observe or address agents, tasks, or PRs of another repo
- **THEN** it cannot; its channel is scoped to its own project

## ADDED Requirements

### Requirement: The planner is a third role that plans, never builds

Sindri SHALL support a third agent role, the planner, alongside worker and
reviewer. A planner SHALL run with the same runtime shape as any agent —
interactive Claude in a named tmux session, driven by the same thin browser
client, woken only by hub injection — but it SHALL NOT be auto-assigned backlog
tasks. The planner's job is to shape upcoming work with the user: read the repo
and specs, propose tasks, and draft openspec. Its briefing SHALL tell it it is
the planner and how its loop differs from a worker's.

#### Scenario: Planner registered as a distinct role

- **WHEN** an agent is registered with the planner role
- **THEN** it is accepted as a valid role distinct from worker and reviewer, and
  launches with the same tmux/browser runtime as any agent

#### Scenario: Planner is never handed a task

- **WHEN** open backlog tasks exist and a planner asks the hub for its next action
- **THEN** the hub does not assign it a task; planners build nothing — only workers
  claim backlog tasks

### Requirement: Planner rests in an orient-and-wait directive

A planner SHALL NOT block waiting for work to be assigned. When it has no
in-flight plan under review, its next-action directive SHALL be to orient — read
README, the backlog, and the specs — and then wait for the user to steer it.
Only an in-flight openspec PR (submitted and awaiting a verdict) SHALL put the
planner into the wait-for-verdict state; everything else returns the orient brief.

#### Scenario: Idle planner is told to orient and wait

- **WHEN** a planner with no submitted plan asks the hub for its next action
- **THEN** the hub returns the orient-and-wait brief rather than a task or a block

#### Scenario: Planner awaiting a verdict

- **WHEN** a planner has shipped an openspec PR that is still under review
- **THEN** its next-action directive is to wait for the verdict, like a worker that
  has submitted
