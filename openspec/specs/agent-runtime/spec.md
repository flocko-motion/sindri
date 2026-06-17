# Agent runtime

## Purpose

Defines how a sindri agent runs: a Claude session inside a named tmux session in
its own pod, driven by a thin role-agnostic client that knows no subcommands of
its own and gets its command surface from the hub. The agent acts, reports, and
goes idle — it never blocks — and is woken only by the hub injecting
provenance-stamped input. This capability covers the agent's runtime shape, its
non-blocking discipline, how humans attach to its live session, and the
isolation that hides its role, the roster, and other agents from it.

## Requirements

### Requirement: Agent runs interactive in a named tmux session

An agent SHALL run Claude interactively inside a tmux session named after the
agent, inside its pod. The hub SHALL deliver inbound messages by sending keys to
that session "as if the user typed them." The session SHALL be attachable and
capturable so its terminal can be observed.

#### Scenario: Session started

- **WHEN** an agent pod launches
- **THEN** Claude runs inside a tmux session named for the agent, ready to receive
  injected input

#### Scenario: Hub delivers a message

- **WHEN** the hub has something for the agent
- **THEN** it sends the message as keystrokes into the agent's tmux session

### Requirement: Thin browser client with no built-in subcommands

The agent's binary SHALL be a single, role-agnostic client that knows no
subcommands of its own. The set of available commands SHALL come from the hub.
Running the client with no arguments SHALL ask the hub for the next action. The
client SHALL NOT execute domain logic locally; it forwards intent to the hub and
relays the result.

#### Scenario: No args asks the hub

- **WHEN** the agent runs its client with no arguments
- **THEN** the hub returns the next action for that agent and the client relays it

#### Scenario: Surface comes from the hub

- **WHEN** the agent lists what it can do
- **THEN** the list is whatever the hub currently permits, not a fixed compiled-in
  command tree

### Requirement: No blocking — act, report, then idle

Every hub call SHALL return promptly. After reporting (for example, registering a
branch for merge), the agent SHALL go idle at its prompt rather than hold a call
open. The agent SHALL be woken only by the hub injecting input, and SHALL be
instructed that waiting is expected and may take a long time.

#### Scenario: Submit returns immediately

- **WHEN** an agent registers its branch for merge
- **THEN** the call returns at once ("registered, please wait") and the agent goes
  idle instead of blocking

#### Scenario: Woken by injection

- **WHEN** the hub has the next task or a verdict
- **THEN** it injects the message into the idle agent's session to resume it

### Requirement: Inbound messages are provenance-stamped

Every message the hub injects into an agent's session SHALL carry a source tag —
at least `[hub]`, `[user]`, and `[reviewer]` — so a single merged input stream is
legible and the agent can weight messages by source.

#### Scenario: Tagged delivery

- **WHEN** the hub injects any message into an agent's session
- **THEN** the message is prefixed with the tag of its originator

### Requirement: A human can attach to the live session

A human SHALL be able to attach to an agent's tmux session and interact with it
directly in a live terminal (for example, `sindri attach <name>` resolving the
name to its pod and attaching). Directly typed input goes straight into the
session and SHALL NOT be routed or stamped by the hub — attach is an out-of-band
override, distinct from the hub-mediated `tell` channel. A read-only attach SHALL
also be possible for observation.

#### Scenario: Dial in

- **WHEN** a user attaches to a running agent's session
- **THEN** they share the agent's live terminal and can type into it directly,
  while the hub may still inject in parallel

#### Scenario: Attach bypasses the hub record

- **WHEN** a human types into a session while attached
- **THEN** that input reaches the agent directly, unstamped and unseen by the hub,
  unlike a message sent via `tell`

### Requirement: Agent sees only its own workspace

An agent SHALL see only its git workspace, named after the agent. Its role, the
roster, the `.sindri/` directory, and other agents' workspaces SHALL NOT be
visible to it.

#### Scenario: Role invisible to the agent

- **WHEN** an agent inspects its environment
- **THEN** it cannot determine whether it is a worker or a reviewer; only the hub
  knows the role
