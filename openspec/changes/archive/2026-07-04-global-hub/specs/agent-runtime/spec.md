## MODIFIED Requirements

### Requirement: Agent sees only its own workspace

An agent SHALL see only its git workspace, named after the agent. Its role, any
roster, the hub's state directory, and other agents' workspaces — in its own project
or any other — SHALL NOT be visible to it. Its sole channel to the hub resolves only
to its own `(project, agent)` identity.

#### Scenario: Role invisible to the agent

- **WHEN** an agent inspects its environment
- **THEN** it cannot determine whether it is a worker or a reviewer; only the hub
  knows the role

#### Scenario: Other projects invisible

- **WHEN** an agent tries to observe or address agents, tasks, or PRs of another repo
- **THEN** it cannot; its channel is scoped to its own project

## ADDED Requirements

### Requirement: Agent-to-hub channel identifies (project, agent)

On Linux the agent SHALL reach the hub over a per-agent unix socket whose path is
its identity. On macOS, where a bind-mounted socket cannot be connected across the
podman VM boundary, the agent SHALL reach the hub over a loopback TCP channel
authenticated by a per-agent bearer token derived as `HMAC(hub-secret, project + name)`;
the hub SHALL resolve a presented token to exactly one `(project, agent)` and reject
any unrecognized token. The agent home and per-agent socket SHALL live under the
central state dir, not in the repo.

#### Scenario: macOS token authenticates the agent

- **WHEN** an agent presents its token over the loopback TCP channel
- **THEN** the hub resolves it to that agent's `(project, name)` and serves its
  surface; a bad or missing token is rejected

#### Scenario: Linux socket identity

- **WHEN** an agent connects over its mounted unix socket on Linux
- **THEN** the hub identifies it by the socket without any token
