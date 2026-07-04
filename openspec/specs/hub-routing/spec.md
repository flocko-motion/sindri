# Hub routing

## Purpose

Defines how messages reach agents: through a single hub-owned injection
primitive, with the hub as the sole holder of the name→pod and object→pod
routing tables derived from `.sindri/`. A human steers any agent by name; agents
never address each other directly but act on shared objects whose consequences
the hub routes to the owning agent. This capability covers the one delivery
primitive, user-to-agent steering, and object-addressed agent-to-agent routing.
## Requirements
### Requirement: One delivery primitive, hub-owned routing

All inbound delivery to an agent SHALL go through the hub's single injection
primitive (sending keys into the agent's session). The hub SHALL be the sole holder
of the `(project, name)→pod` and `(project, object)→pod` routing tables, derived
from its central store. No other actor SHALL resolve an agent to its pod, and
resolution SHALL be scoped to the agent's project.

#### Scenario: Hub routes by (project, name)

- **WHEN** a message is addressed to an agent in a given project
- **THEN** the hub resolves it within that project to the agent's pod and injects
  the message

### Requirement: User can steer any agent

The host CLI SHALL let a user send a message to any agent by name within a repo
context (for example, `sindri tell <name> "..."` run in that repo). The message
SHALL be delivered into that agent's session, stamped `[user]`.

#### Scenario: Hello to an agent

- **WHEN** a user runs `sindri tell <name> "hello"` in the agent's repo
- **THEN** `[user] hello` appears in that agent's session

#### Scenario: Unknown agent

- **WHEN** a user addresses a name with no entry in that project's roster
- **THEN** the hub reports that no such agent exists and delivers nothing

### Requirement: Agent-to-agent is object-addressed, never peer-addressed

An agent SHALL NOT address another agent directly. An agent acts on a shared
object (a PR or task), and the hub SHALL route the consequence to the agent that
owns that object. The reviewer rejecting a PR SHALL cause the hub to resolve the
PR's branch to its owning agent and inject the feedback there, stamped
`[reviewer]`.

#### Scenario: Reviewer feedback reaches the worker

- **WHEN** the reviewer rejects a PR with feedback
- **THEN** the hub resolves the PR's branch to its owning agent and injects the
  feedback into that agent's session, tagged `[reviewer]`

#### Scenario: No peer addressing

- **WHEN** an agent tries to send a message to another agent by name
- **THEN** it cannot — agents have no roster visibility, and routing is the hub's
  alone

