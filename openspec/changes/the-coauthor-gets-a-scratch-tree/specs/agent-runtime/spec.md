# agent-runtime — delta

## MODIFIED Requirements

### Requirement: Agent sees only its own workspace

An agent SHALL see only its own git workspace and the single hub socket that is
its channel to the hub; the roster, the hub's durable state, and other agents'
workspaces SHALL NOT be visible to it. The hub's state — its database, its logs, the agent homes —
lives outside every repository, so no mount shields it: it is not there to find. A project's own
`.sindri/config.yaml` is a tracked file of that repository and is visible like any other, which is
not a leak of anything the hub owns.

For a worker or reviewer the workspace is an isolated per-agent worktree named after the agent. The
coauthor is the deliberate exception: because it works the same material as the user, its workspace IS
the user's own working checkout (the repo root), so it is NOT isolated from the user's tree. Even
then, the directory holding the other agents' worktrees SHALL be hidden from it, overlaid by an empty
read-only directory — the hub commits from those trees, so an edit made in one would land in another
agent's pull request as that agent's work.

The hub MAY tell an agent its own role — it briefs the agent in a role-specific way and the `status`
verb reports the role — but an agent SHALL NOT be able to enumerate the roster, or address or observe
another agent.

#### Scenario: Agent knows its own role but not the roster

- **WHEN** an agent inspects its environment
- **THEN** it knows its own role from the hub's briefing, but it cannot enumerate
  the roster or see other agents' workspaces

#### Scenario: No cross-agent visibility

- **WHEN** an agent tries to discover or address another agent
- **THEN** it cannot — only the hub holds the roster and the routing tables

#### Scenario: Isolated agent sees only its own worktree

- **WHEN** a worker or reviewer inspects its filesystem
- **THEN** it finds its own worktree and its hub socket, and neither the roster nor another agent's
  workspace

#### Scenario: Coauthor shares the user's checkout but not .sindri

- **WHEN** a coauthor inspects its filesystem
- **THEN** its `/workspace` is the user's actual repository checkout (edits are shared with the user),
  the directory holding the other agents' worktrees is empty and read-only, and the only `.sindri/`
  there is the project's own config (the scenario keeps its title because renaming one drops it)

#### Scenario: Other projects invisible

- **WHEN** an agent tries to observe or address agents, tasks, or PRs of another repo
- **THEN** it cannot; its channel is scoped to its own project

## ADDED Requirements

### Requirement: The hub checks work out into the coauthor's scratch tree

A coauthor SHALL be able to ask the hub to check a branch, a commit or a pull request out into its
scratch workspace, and SHALL build and test there. The hub performs the checkout: agents have no git
of their own beyond the curated surface, and this is the same in-place detached checkout that puts a
pull request into a reviewer's live workspace.

A pull request id SHALL be accepted wherever a ref is, resolved to its branch by the hub. Asking to
see a PR is the common case, and resolving it by hand is friction the hub can absorb.

The checkout SHALL be detached. The branch is checked out in its author's worktree and git gives a
branch to one worktree at a time, so a branch checkout would fail — and a commit is the right thing to
inspect regardless.

A checkout that would discard uncommitted changes in the scratch tree SHALL be refused, naming both
ways forward: move the work into the shared checkout, or repeat the request with an explicit force.
Nothing in that tree is recorded anywhere, so the refusal is the only warning there is; silently
discarding it would lose exactly the half-finished experiment the tree exists for.

#### Scenario: Asking for a pull request by id

- **WHEN** a coauthor asks the hub for a pull request id
- **THEN** that PR's branch is checked out detached in its scratch workspace, and it can build and
  test it there

#### Scenario: A dirty scratch tree

- **WHEN** a coauthor has uncommitted changes in its scratch workspace and asks for another checkout
- **THEN** the request is refused with the work intact, and the reply names how to keep it and how to
  discard it

#### Scenario: Forcing the checkout

- **WHEN** the coauthor repeats the request with force
- **THEN** the scratch workspace is replaced by the requested state
