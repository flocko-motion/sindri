# hub — delta

## ADDED Requirements

### Requirement: `_global` is a virtual project for the fleet-wide reviewer pool

The hub SHALL register a virtual project `_global` at startup — not lazily on first use like a real
repo, since no client request ever names it as a path to register. Its registry path SHALL be a
dedicated directory under the state directory whose basename is literally `_global`, so every
existing project-keyed mechanism (container naming, the socket and home paths under the state dir)
works unchanged with no special-casing for it.

`_global` SHALL NOT be forgettable (`repo forget`): it holds no worktrees, and the hub re-registers
it on its next restart regardless, so forgetting it would only tear down its reviewers for no lasting
effect. A host request naming `_global` explicitly (rather than a filesystem path) SHALL resolve to
that literal project tag directly, never be hashed as though it were a path — hashing it would
silently register an unrelated phantom project instead of reaching the pool.

#### Scenario: `_global` exists before any request names it

- **WHEN** the hub starts
- **THEN** `_global` is already a registered project, with no repo directory required to exist

#### Scenario: `_global` cannot be forgotten

- **WHEN** a user runs `repo forget` naming `_global`
- **THEN** the hub refuses, naming what it is instead (no worktrees, not a repo)

#### Scenario: A request naming `_global` resolves to it exactly

- **WHEN** a host request's project header is the literal string `_global`
- **THEN** the hub resolves it to the `_global` project directly, registering no other project as a
  side effect
