# GitHub Issues — delta

## ADDED Requirements

### Requirement: GitHub issues are a todo source through an adapter

Sindri SHALL be able to import a repository's open GitHub issues as tasks in the
hub's cached backlog, alongside td tasks and openspec changes. All access to
GitHub SHALL go through a dedicated adapter (`internal/adapter/github`) that
shells out to the `gh` CLI — reusing the user's existing `gh` authentication —
and the adapter SHALL be stateless package-level functions that import nothing
from the hub, store, or issue-model packages (the same adapter shape as `td` and
`spec`).

#### Scenario: Open issues appear as tasks

- **WHEN** the hub syncs a project with the GitHub source enabled
- **THEN** each open GitHub issue in the repo's remote appears as a task in the
  cached backlog

#### Scenario: All access via the adapter

- **WHEN** the hub reads issues or closes one
- **THEN** it calls the `internal/adapter/github` package, which invokes `gh` —
  the hub never contacts the GitHub API directly

### Requirement: All open issues are imported; pull requests excluded

The source SHALL import every open issue in the repo's GitHub remote — there is
no label or assignee filter. Pull requests SHALL NOT be imported (they are not
todos and are handled by the local PR workflow).

#### Scenario: Every open issue imported

- **WHEN** the repo has open issues with assorted (or no) labels
- **THEN** all of them are imported as tasks, regardless of labels

#### Scenario: Pull requests are not tasks

- **WHEN** the repo has open pull requests
- **THEN** none of them appear as tasks (only issues do)

### Requirement: Stable `gh-<number>` identity

Each imported issue SHALL be identified by `gh-<number>`, where `<number>` is the
GitHub issue number. The id SHALL be stable across resyncs for the life of the
issue, and the task's type SHALL be `issue`.

#### Scenario: Id derived from issue number

- **WHEN** issue #123 is imported
- **THEN** its task id is `gh-123` and its type is `issue`

#### Scenario: Id stable across resyncs

- **WHEN** the same issue is synced again
- **THEN** it keeps the id `gh-123` (no duplicate row is created)

### Requirement: Imported issues are directly claimable

An imported issue SHALL be an open, childless leaf and SHALL be eligible for the
hub's normal auto-assignment with no extra approval gate — a worker claims it,
branches, builds, and opens a local PR exactly as for a td task.

#### Scenario: Worker claims an issue

- **WHEN** a free worker asks for the next task and the highest-priority open
  leaf is `gh-123`
- **THEN** the worker claims `gh-123`, works on a `gh-123` branch in its worktree,
  and can register the branch for merge — no approval step is required first

### Requirement: New issues default to the lowest priority tier

GitHub issues carry no native priority. A newly imported issue SHALL be assigned
the lowest priority tier by default, and a human SHALL be able to re-rate it via
the hub's existing priority override (the same mechanism used for openspec items).
The lowest priority tier's display word SHALL be `none` (renamed from `trivial`);
`trivial` SHALL still be accepted as an input alias for that tier.

#### Scenario: New issue comes in as none

- **WHEN** issue #123 is first imported
- **THEN** its priority is the lowest tier, displayed as `none`

#### Scenario: Human re-rates an imported issue

- **WHEN** a human sets `gh-123` to a higher priority
- **THEN** the override is stored hub-side and the issue sorts at the new priority,
  and this override survives subsequent resyncs

#### Scenario: Lowest tier reads "none" everywhere

- **WHEN** any task at the lowest priority tier is displayed
- **THEN** its priority word is `none`, not `trivial`

### Requirement: Close and comment the issue on merge

When the local PR for a `gh-*` task is merged, the hub SHALL close the
corresponding GitHub issue and leave a comment noting the merge. This write-back
SHALL be best-effort: if GitHub is unreachable or the close fails, the local
merge SHALL still succeed and the failure SHALL be surfaced as a warning — the
merge is never blocked on the network or on GitHub.

#### Scenario: Merge closes the issue

- **WHEN** a worker's PR for `gh-123` is merged and GitHub is reachable
- **THEN** issue #123 is closed on GitHub with a comment noting the merge

#### Scenario: Merge succeeds when GitHub is unreachable

- **WHEN** a worker's PR for `gh-123` is merged but `gh` cannot reach GitHub
- **THEN** the local merge still completes and the failed write-back is reported as
  a warning (the issue stays open on GitHub)

### Requirement: Opt-in per project, absent when GitHub is unavailable

The GitHub source SHALL be disabled by default and enabled explicitly per
project. Even when enabled, it SHALL degrade to absent — importing no issues and
never raising a hard error — whenever `gh` is not installed, the user is not
authenticated, the repository has no GitHub remote, or the network is
unreachable. Absence of the source SHALL NOT affect td tasks, openspec changes,
or any offline workflow.

#### Scenario: Disabled by default

- **WHEN** a project has a GitHub remote but has not enabled the source
- **THEN** no issues are imported

#### Scenario: Enabled but gh unavailable

- **WHEN** the source is enabled but `gh` is missing / unauthenticated / offline /
  the repo has no GitHub remote
- **THEN** the sync completes with no GitHub tasks and no error, and td + openspec
  tasks are unaffected

### Requirement: An issue closed outside sindri leaves the backlog on resync

An issue closed on GitHub directly (outside sindri) SHALL disappear from the
backlog on the next resync, because the cache mirrors the source and only open
issues are imported. An in-flight `gh-*` task already claimed by a worker SHALL
NOT be interrupted by its source issue vanishing; the worker's branch and PR
proceed as normal.

#### Scenario: Issue closed on GitHub

- **WHEN** issue #123 is closed on GitHub and the hub resyncs
- **THEN** `gh-123` no longer appears among open tasks

#### Scenario: Claimed issue closed upstream mid-flight

- **WHEN** a worker has already claimed `gh-123` and the issue is closed on GitHub
- **THEN** the worker's in-progress branch and PR are unaffected
