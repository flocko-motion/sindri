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

### Requirement: Container runtime is a pluggable backend behind one port

The hub SHALL reach the container runtime through a single port (interface), with interchangeable backends, so that no hub, CLI, or TUI code depends on a specific runtime CLI. At least two backends SHALL satisfy the port: podman (a shared Linux VM) and Apple `container` (a per-container micro-VM, macOS). The backend SHALL be selectable by configuration. On macOS the default SHALL be Apple `container` — its per-agent micro-VM is what satisfies the isolation requirement below — with podman available as an opt-out (`SINDRI_RUNTIME=podman`); on Linux the backend SHALL always be podman, since Apple `container` requires macOS.

#### Scenario: A caller drives the runtime through the port

- **WHEN** the hub launches, execs into, attaches to, or tears down an agent's pod
- **THEN** it calls the runtime port, and the concrete backend (podman or Apple `container`) is chosen once from configuration — no caller references a specific runtime CLI

#### Scenario: Backend is swapped without touching callers

- **WHEN** the configured runtime backend changes
- **THEN** only the backend selection changes; the hub, CLI, and TUI code is unaffected because they use the port

#### Scenario: Default and platform constraint

- **WHEN** no runtime is configured on macOS
- **THEN** the Apple `container` backend is used (opt out with `SINDRI_RUNTIME=podman`)
- **WHEN** the host is Linux, or podman is explicitly selected
- **THEN** the podman backend is used

### Requirement: One agent's runtime failure is isolated from other agents

An agent SHALL run in its own runtime instance such that the crash, out-of-memory, or wedge of one agent's runtime does not take down other agents. A backend that shares a single VM across all agents does not satisfy this (one VM failure ends the whole fleet); a backend that gives each container its own micro-VM does.

#### Scenario: One agent's runtime dies

- **WHEN** a single agent's runtime instance crashes or is OOM-killed
- **THEN** the other agents keep running, and only the failed agent is reported down

#### Scenario: Shared-VM backend is a known limitation

- **WHEN** the shared-VM backend (podman) is in use and its VM fails
- **THEN** the fleet-wide failure is understood as that backend's limitation — the per-container-VM backend exists to avoid it

