# gh-local

## Purpose

Defines how sindri does pull requests and branches entirely locally — no
GitHub, no remote. Agents drive a familiar `gh`-style workflow; PRs are records
under `.git/`, and each task is developed on its own branch in an isolated
worktree. This is the spec for the local PR/worktree machinery; the agent loop
that uses it is in workers, and the human review flow is a separate action spec.

## Requirements

### Requirement: Local-only, not GitHub

The `gh` command provided to agents SHALL be sindri-local, operating only on the
local repository. It SHALL NOT contact GitHub or any remote, and every
subcommand SHALL make clear it is not the GitHub CLI.

#### Scenario: Unknown command

- **WHEN** an agent runs an unsupported `gh` command
- **THEN** it is told this is sindri-local (not GitHub) and shown the real commands

### Requirement: PRs are local records

A pull request SHALL be stored as a record under the repository's `.git/`
(branch, base, status, title, body, diff). PR state SHALL be one of open,
approved, rejected, or merged. PRs SHALL be keyed by task id so a task's PRs can
be found, with later revisions suffixed when an earlier PR for the task is gone.

#### Scenario: Creating a PR

- **WHEN** an agent submits work for a task
- **THEN** a PR record is written under `.git/` in the open state, keyed by the task

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

### Requirement: Self-contained, no remote dependency

The whole PR/worktree workflow SHALL function with no network and no GitHub
account; everything lives in the local git repository.

#### Scenario: Offline

- **WHEN** sindri runs with no network
- **THEN** create, review, and merge of local PRs all still work

## Structure

- `internal/ghlocal/store/` (`type: adapter`) — the PR record store and the
  git checkout/merge/branch operations.
- `cmd/gh/` (`type: command`) — the agent-facing `gh` workflow CLI
  (issue/submit/done/pr…), wrapping the store, td, and git.
- `internal/worker/` (`type: adapter`) — creates and tends the worktrees the
  branches live in (see workers).
