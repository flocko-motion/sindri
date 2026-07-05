# Hub — delta

## MODIFIED Requirements

### Requirement: One project-keyed store, no per-repo state files

The hub SHALL keep all its state in a single store under the central state dir,
with every per-repo row tagged by a project key (`repoTag`, a stable digest of the
repo's absolute path). Agent identity SHALL be unique per `(project, name)`, so the
same agent name MAY exist in different repos. The hub SHALL NOT write any state into
the repositories it serves; a repo's only sindri-related on-disk content is
git-owned worktrees and td's own `.todos/` — both gitignored by the hub, never
committed.

#### Scenario: Same agent name in two repos

- **WHEN** two different repos each register an agent named "eitri"
- **THEN** both exist as distinct `(project, name)` identities and never collide

#### Scenario: Task data is never committed

- **WHEN** the hub first serves a repo
- **THEN** it ensures the repo's `.gitignore` lists both `.worktrees/` and
  `.todos/`, so the constantly-rewritten task DB can never be committed and collide
  with the host checkout's live `.todos/` at merge time
