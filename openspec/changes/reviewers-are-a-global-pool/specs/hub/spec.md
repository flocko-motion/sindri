# hub — delta

## ADDED Requirements

### Requirement: A virtual project holds agents that belong to no repo

The hub SHALL register a project tagged `_global`, whose path is the state dir. It is a project in
every mechanical sense — the store keys rows `(project, name)`, the socket and home follow the
per-project layout, and the container name is built the same way — so an agent living there needs no
schema of its own.

The tag SHALL be `_global` rather than `global` or a sigil form. Generated tags are eight hex
characters, so `global` cannot collide with one; the collision worth guarding is a repo directory of
that name reaching the same value through the CLI's name lookup. `repoSlug` keeps only
`[a-z0-9-_]` when composing a container name, so `$global` and `*global` are silently stripped to
`global` and collide with exactly what they were meant to distinguish. Underscore survives, and is
shell-safe where `*` globs and `$` expands.

`_global` SHALL NOT be offered as a repo. It accepts no worktrees, appears in no list of repos to
work in, and is refused where a command asks which repo to act on.

#### Scenario: The virtual project resolves like any other

- **WHEN** the hub starts
- **THEN** `_global` is in the registry with the state dir as its path, and an agent created there
  gets a store row, a socket and a home by the same rules a repo-bound agent gets them

#### Scenario: It is not somewhere to work

- **WHEN** a command asks which repo to act on, or lists repos to work in
- **THEN** `_global` is refused and does not appear

#### Scenario: A sigil would collide where it matters

- **WHEN** a container name is composed for a project tagged `$global` or `*global`
- **THEN** the sigil is stripped and the name collides with a repo directory named `global`, which is
  why the tag is `_global`

### Requirement: Global agents are in scope everywhere

An agent in `_global` SHALL be in scope in every project, in every front-end. It is not a foreign row
— foreign means "waiting on you in another repo", and an agent belonging to no repo is not elsewhere.

Both front-ends SHALL show global agents in their ordinary place rather than in a foreign section,
whatever repo the user is scoped to, and count them in the same tallies as local ones.

#### Scenario: Scoped to a repo, the global pool is still shown

- **WHEN** the user is scoped to any repo and the board is rendered
- **THEN** `_global` agents appear in the ordinary rows, not a foreign section, and count in the same
  tallies as local agents
