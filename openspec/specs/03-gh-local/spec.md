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

A pull request SHALL be a merge-intent owned by the hub: a branch, a flag meaning
"the agent would like this branch merged," and a verdict. The hub SHALL hold this
state; agents SHALL NOT write a PR store. There SHALL NOT be a separate `.git/pr`
record store.

#### Scenario: Registering merge-intent

- **WHEN** an agent registers its branch for merge
- **THEN** the hub records the intent (branch + wants-merge + pending verdict) in
  its own state, and the call returns immediately

### Requirement: Lint gate before submit

Submitting (and creating a PR) SHALL run the project's quality gates after the
rebase and before the PR record is written — the same gates as `sindri lint all`
(file length, dead code, and OpenSpec validation). If any violation is found, or
a gate cannot run (e.g. the code does not compile), the submit SHALL be refused
and the violations reported, so a failing PR is never created. OpenSpec
validation SHALL be skipped when the project doesn't use openspec.

#### Scenario: Clean submit

- **WHEN** an agent submits work that passes every gate
- **THEN** the PR record is created

#### Scenario: Lint violation

- **WHEN** an agent submits work that fails a gate (lint or an invalid spec)
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

The agent client SHALL be a single role-agnostic browser whose available commands
are filtered by the hub from the caller's role and state. A worker's surface SHALL
expose registering and inspecting merge-intents but never approve/reject/merge; a
reviewer's surface SHALL expose approve/reject but never submit. Merge SHALL be
human-only, exposed only on the host and requiring explicit confirmation; no agent
surface SHALL ever include merge.

#### Scenario: Reviewer approves, human merges

- **WHEN** the reviewer approves a PR
- **THEN** the hub marks it approved and its gates satisfied, but it is merged only
  later by a human on the host

#### Scenario: No agent merge

- **WHEN** any agent queries its command surface
- **THEN** no merge command appears; only the host `sindri pr merge` can merge,
  after human confirmation

### Requirement: Self-contained, no remote dependency

The whole PR/worktree workflow SHALL function with no network and no GitHub
account; everything lives in the local git repository.

#### Scenario: Offline

- **WHEN** sindri runs with no network
- **THEN** create, review, and merge of local PRs all still work

### Requirement: td reads are direct, writes go through the tool

For the td backend, the td adapter SHALL read tasks directly from td's own SQLite
database for speed, but SHALL perform every write action (create, start, comment,
review, …) only through the `td` tool — never by writing td's database directly.
Both strategies SHALL be encapsulated in `internal/adapter/td` so internal logic
sees a single adapter interface.

#### Scenario: Fast read

- **WHEN** the hub syncs td tasks into its cache
- **THEN** it reads td's SQLite directly rather than invoking the td CLI per query

#### Scenario: Write through the tool

- **WHEN** a td task is created or mutated
- **THEN** the change goes through the `td` tool, never a direct write to td's DB

## Structure

- `internal/ghlocal/store/` (`type: adapter`) — the PR record store and the
  git checkout/merge/branch operations.
- `internal/agentcli/` (`type: command`) — the shared agent command set
  (issue/submit/done/pr create/list/view, plus pr approve/reject for review),
  wrapping the store, td, git, and the lint gate. Two thin entrypoints wire role
  subsets: `cmd/sindri-worker/` (worker) and `cmd/sindri-review/` (reviewer).
- `internal/worker/` (`type: adapter`) — creates and tends the worktrees the
  branches live in (see workers).
