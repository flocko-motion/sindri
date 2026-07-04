## MODIFIED Requirements

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
