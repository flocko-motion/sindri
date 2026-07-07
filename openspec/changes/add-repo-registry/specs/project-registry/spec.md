# project-registry — delta

## MODIFIED Requirements

### Requirement: Global hub tracks known projects

The hub SHALL maintain a registry mapping each known project's stable key
(`repoTag`, a digest of the repo's absolute path) to its repository root path and a
`last used` timestamp. A repo SHALL be registered on first use — the first request
that carries its context — and MAY additionally be registered up front by an
explicit setup step (`repo init`); explicit registration is **additive** and never a
precondition for use. The registry SHALL be the source of truth for resolving a
project key to a path, for ordering the repo switcher (see `view-tui`), and for
listing projects to the UI. The registry SHALL support dropping a project's row
(`repo forget`) without touching the repository's files.

#### Scenario: Repo registered on first use

- **WHEN** a client first sends a request carrying a repo not yet known
- **THEN** the hub records that repo's `repoTag → path` in the registry

#### Scenario: Explicit registration is additive

- **WHEN** a repo that was never explicitly set up is used
- **THEN** it still self-registers on first use, exactly as an explicitly-registered
  repo would — `repo init` is a convenience, not a prerequisite

#### Scenario: Listing projects

- **WHEN** the TUI or CLI asks for the set of known projects
- **THEN** the hub returns each project's key, human-readable repo path, and the
  data needed to order them (live-agent presence, last-used)

#### Scenario: Resolving a project key

- **WHEN** the hub needs the on-disk path for a project key (e.g. to create a
  worktree in that repo)
- **THEN** it resolves the path via the registry

## ADDED Requirements

### Requirement: Explicit repo setup (`repo init`)

The hub SHALL offer an explicit repo setup that (1) registers the repo eagerly,
(2) scaffolds a committed `.sindri/config.yaml` when absent, and (3) seeds the
architecture doc when the project has not configured its own. Setup SHALL be
idempotent and non-destructive: it never overwrites an existing `.sindri/config.yaml`
and never requires the repo to be unregistered first. Setup SHALL NOT be a
precondition for any other operation.

#### Scenario: Init a fresh repo

- **WHEN** the user runs `repo init` in a repo with no `.sindri/config.yaml`
- **THEN** the repo is registered and a commented `.sindri/config.yaml` template is
  written, plus a seeded `ARCHITECTURE.md` if the project has none

#### Scenario: Init is idempotent

- **WHEN** the user runs `repo init` in an already-registered repo that already has
  a `.sindri/config.yaml`
- **THEN** the registration is refreshed and the existing config file is left
  untouched (never clobbered)

### Requirement: Repo listing and inspection (`repo list`, `repo info`)

The hub SHALL expose the registry through a listing (every registered repo with its
key, path, live-agent count, and source/config flags) and a per-repo inspection (one
repo's resolved config and its agent/PR/task counts). These reads SHALL be available
to both the CLI and the TUI.

#### Scenario: List registered repos

- **WHEN** the user runs `repo list`
- **THEN** every registered repo is shown with its path and a summary (e.g. how many
  agents it has)

#### Scenario: Inspect one repo

- **WHEN** the user runs `repo info` (defaulting to the current repo) or `repo info
  <repo>`
- **THEN** that repo's resolved configuration and its counts are shown

### Requirement: Give up management without deleting (`repo forget`)

The hub SHALL let a user drop a repo from the registry (`repo forget`). Forgetting
SHALL remove only the registry row — the repository, its `.sindri/` files, its
worktrees, and its git history SHALL NOT be touched. Forgetting SHALL be refused
while the repo still has agents on its roster; the user must stop or delete those
agents first. Because implicit registration remains, a forgotten repo SHALL
re-register on next use — `forget` drops the repo from the current view, it does not
permanently exclude it.

#### Scenario: Forget an idle repo

- **WHEN** the user runs `repo forget <repo>` on a repo with no agents
- **THEN** the registry row is removed and the repo's files on disk are unchanged

#### Scenario: Forget refused while agents live

- **WHEN** the user runs `repo forget <repo>` and that repo still has agents
- **THEN** the operation is refused with a message telling them to stop or delete the
  agents first, and the registry row is kept

#### Scenario: Forgotten repo returns on use

- **WHEN** a forgotten repo is used again by any sindri command
- **THEN** it silently re-registers (forget is not a permanent ban)
