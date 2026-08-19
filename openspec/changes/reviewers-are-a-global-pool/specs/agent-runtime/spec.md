# agent-runtime — delta

## ADDED Requirements

### Requirement: A global agent's pod carries no repo

A pod for an agent in `_global` SHALL mount no repository. Its `/workspace` is the fixed path the hub
materialises into, and the mounts a repo-bound agent gets for its worktree and scratch tree have
nothing to point at.

Everything the pod needs that is not the repo SHALL be unchanged: the toolbelt, the agent binary, the
credentials the hub stages, and the skills and keybindings an agent inherits are all properties of
the install rather than of a project.

Where a repo's own content is needed to judge a PR — its architecture doc among it — the hub SHALL
carry it into the assignment as text, as it already does. A reviewer reads what it is given rather
than reaching into a repo it no longer mounts.

#### Scenario: The pod mounts no repository

- **WHEN** a `_global` agent's pod is launched
- **THEN** it mounts its own fixed workspace and the install's own tooling, and no repository

#### Scenario: The repo's own guidance still reaches it

- **WHEN** a review is assigned for a repo with an architecture doc
- **THEN** the doc's content reaches the reviewer in the assignment
