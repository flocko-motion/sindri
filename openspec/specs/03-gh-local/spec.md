# gh-local

## Purpose

Defines how sindri does pull requests and branches entirely locally — no
GitHub, no remote. Agents drive the sindri-local workflow CLIs (`sindri-worker`
for the worker, `sindri-review` for the reviewer — deliberately not named `gh`,
so they are never mistaken for the GitHub CLI); PRs are records under `.git/`,
and each task is developed on its own branch in an isolated worktree. This is
the spec for the local PR/worktree machinery; the agent loop that uses it is in
workers, and the human review flow is a separate action spec.

## Requirements

### Requirement: Local-only, not GitHub

The agent CLIs (`sindri-worker`, `sindri-review`) SHALL be sindri-local,
operating only on the local repository. They SHALL NOT contact GitHub or any
remote, and every subcommand SHALL make clear it is not the GitHub CLI.

#### Scenario: Unknown command

- **WHEN** an agent runs an unsupported command on either CLI
- **THEN** it is told this is sindri-local (not GitHub) and shown the real commands

### Requirement: PRs are local records

A pull request SHALL be stored as a record under the repository's `.git/`
(branch, base, status, title, body, diff). PR state SHALL be one of open,
approved, rejected, or merged. PRs SHALL be keyed by task id so a task's PRs can
be found, with later revisions suffixed when an earlier PR for the task is gone.

#### Scenario: Creating a PR

- **WHEN** an agent submits work for a task
- **THEN** a PR record is written under `.git/` in the open state, keyed by the task

### Requirement: Lint gate before submit

Submitting (and creating a PR) SHALL run the project linters after the rebase
and before the PR record is written; if any violation is found, or a linter
cannot run (e.g. the code does not compile), the submit SHALL be refused and the
violations reported, so an unlinted PR is never created.

#### Scenario: Clean submit

- **WHEN** an agent submits work that passes lint
- **THEN** the PR record is created

#### Scenario: Lint violation

- **WHEN** an agent submits work that fails lint
- **THEN** no PR is created and the violations are shown for the agent to fix

### Requirement: Per-task branches in worktrees

Each task SHALL be developed on its own branch named for the task, in an
isolated git worktree. A worktree SHALL never check out a branch already in use
by another worktree; the shared base branch is used via detached HEAD.

#### Scenario: Picking up a task

- **WHEN** an agent starts a task
- **THEN** it works on a `td-……` branch created from the base in its own worktree

#### Scenario: Base branch in use

- **WHEN** a worktree needs the base branch that the main repo already checks out
- **THEN** it uses a detached HEAD at the base tip rather than checking out the branch

### Requirement: Merge into base, on approval only

Merging a PR SHALL fast-forward-free merge its branch into the base branch and
mark the PR merged. A PR SHALL only be merged after it is approved, and merging
SHALL be gated by the task's review gates.

#### Scenario: Gated merge

- **WHEN** a merge is attempted while the task has an unmet review gate
- **THEN** the merge is refused until the gate is satisfied

### Requirement: Role-scoped commands; merge is human-only

Each agent role's CLI SHALL expose only the commands that role needs. The worker
CLI (`sindri-worker`) creates/lists/views PRs but cannot approve, reject, or
merge. The reviewer CLI (`sindri-review`) adds `pr approve` and `pr reject` —
the reviewer agent's job, taken without a human gate. Merging SHALL be
human-only: it is exposed only on the host (`sindri pr merge`) and SHALL require
explicit human confirmation; no agent CLI provides a merge command.

#### Scenario: Reviewer approves, human merges

- **WHEN** the reviewer agent approves a PR with `sindri-review pr approve`
- **THEN** the PR is approved and its review gates satisfied, but it is merged
  only later by a human on the host

#### Scenario: No agent merge

- **WHEN** any agent tries to merge
- **THEN** no `sindri-worker`/`sindri-review` merge command exists; only the
  host `sindri pr merge` can, after human confirmation

### Requirement: Self-contained, no remote dependency

The whole PR/worktree workflow SHALL function with no network and no GitHub
account; everything lives in the local git repository.

#### Scenario: Offline

- **WHEN** sindri runs with no network
- **THEN** create, review, and merge of local PRs all still work

## Structure

- `internal/ghlocal/store/` (`type: adapter`) — the PR record store and the
  git checkout/merge/branch operations.
- `internal/agentcli/` (`type: command`) — the shared agent command set
  (issue/submit/done/pr create/list/view, plus pr approve/reject for review),
  wrapping the store, td, git, and the lint gate. Two thin entrypoints wire role
  subsets: `cmd/sindri-worker/` (worker) and `cmd/sindri-review/` (reviewer).
- `internal/worker/` (`type: adapter`) — creates and tends the worktrees the
  branches live in (see workers).
