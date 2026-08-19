# view-workers — delta

## ADDED Requirements

### Requirement: An agent's status never reads "full"

An agent's status word SHALL NOT read a distinct value for context fullness, since fullness no
longer waits on the user: an idle ask clears and reassigns a full worker automatically. An agent's
context fill SHALL remain readable as its numeric fields (`ContextTokens`/`ContextWindow`) whatever
its status reads, so the raw figure stays visible without a status word standing in for it.

#### Scenario: A full, idle agent reads idle

- **WHEN** an agent past the context threshold holds no task, feature or PR
- **THEN** its status reads `idle`, not a distinct "full" value

#### Scenario: Fullness never counts toward needing the user

- **WHEN** the board counts agents that need the user
- **THEN** an agent's context fill by itself is never a reason it is counted

#### Scenario: The raw fill stays visible

- **WHEN** a front-end renders an agent whatever its status
- **THEN** its context token count and window are still readable from the agent's own fields
