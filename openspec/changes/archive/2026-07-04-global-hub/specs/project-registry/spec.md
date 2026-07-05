## ADDED Requirements

### Requirement: Global hub tracks known projects

The hub SHALL maintain a registry mapping each known project's stable key
(`repoTag`, a digest of the repo's absolute path) to its repository root path. A repo
SHALL be registered on first use — the first request that carries its context. The
registry SHALL be the source of truth for resolving a project key to a path and for
listing projects to the UI and the repo switcher.

#### Scenario: Repo registered on first use

- **WHEN** a client first sends a request carrying a repo not yet known
- **THEN** the hub records that repo's `repoTag → path` in the registry

#### Scenario: Listing projects

- **WHEN** the TUI or CLI asks for the set of known projects
- **THEN** the hub returns each project's key and human-readable repo path

#### Scenario: Resolving a project key

- **WHEN** the hub needs the on-disk path for a project key (e.g. to create a
  worktree in that repo)
- **THEN** it resolves the path via the registry
