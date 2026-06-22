# Agent runtime — delta

## MODIFIED Requirements

### Requirement: Agent sees only its own workspace

An agent SHALL see only its git workspace, named after the agent. The roster, the
`.sindri/` directory, and other agents' workspaces SHALL NOT be visible to it. The
hub MAY tell an agent its own role — it briefs the agent in a role-specific way
and the `status` verb reports the role — but an agent SHALL NOT be able to
enumerate the roster, address or observe another agent, or read `.sindri/`.

#### Scenario: Agent knows its own role but not the roster

- **WHEN** an agent inspects its environment
- **THEN** it knows its own role from the hub's briefing, but it cannot enumerate
  the roster, see other agents' workspaces, or read `.sindri/`

#### Scenario: No cross-agent visibility

- **WHEN** an agent tries to discover or address another agent
- **THEN** it cannot — only the hub holds the roster and the routing tables

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
