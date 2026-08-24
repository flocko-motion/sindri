# agent-runtime — delta

## MODIFIED Requirements

### Requirement: Agent sees only its own workspace

An agent SHALL see only its own workspace and the single hub socket that is
its channel to the hub; the roster, the `.sindri/` directory, and other agents'
workspaces SHALL NOT be visible to it. For a worker or a project-bound reviewer the
workspace is an isolated per-agent git worktree named after the agent. The coauthor is
a deliberate exception: because it works the same material as the user, its
workspace IS the user's own working checkout (the repo root), so it is NOT
isolated from the user's tree — but even then the `.sindri/` directory SHALL
remain hidden from it (overlaid by an empty read-only directory), so it can
neither read nor corrupt hub state in the shared checkout. A reviewer living in
the fleet-wide pool (`_global`) is a second exception: it holds no git repository
at all, so its workspace is a fixed directory materialised with a PR's tree as
plain files per review, never a worktree — the hub's curated git surface and the
lint gate run hub-side for it exactly as for any reviewer. Its pod is built from
the install's own default image, never the reviewed repo's — one pod serving
every repo cannot carry every repo's image without the restart this design
exists to avoid — and the lint gate running hub-side is what keeps that
acceptable, since it is the reviewer's main verification tool. The hub MAY tell an
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

- **WHEN** a worker or a project-bound reviewer inspects its filesystem
- **THEN** it finds its own worktree and its hub socket, and neither the roster,
  `.sindri/`, nor another agent's workspace

#### Scenario: Coauthor shares the user's checkout but not .sindri

- **WHEN** a coauthor inspects its filesystem
- **THEN** its `/workspace` is the user's actual repository checkout (edits are
  shared with the user), yet `.sindri/` is still hidden, so it cannot read or
  write hub state

#### Scenario: A pooled reviewer's workspace holds no repository

- **WHEN** a `_global` reviewer inspects its filesystem
- **THEN** `/workspace` holds the assigned PR's files but no `.git` and no other
  repository trace, and no worktree or scratch mount is present

#### Scenario: A pooled reviewer's pod runs the install's own image

- **WHEN** a `_global` reviewer's pod is built
- **THEN** it is built from the install's default image, not the reviewed repo's, and the lint
  gate it relies on to verify a PR still runs hub-side regardless

#### Scenario: Other projects invisible

- **WHEN** an agent tries to observe or address agents, tasks, or PRs of another repo
- **THEN** it cannot; its channel is scoped to its own project
