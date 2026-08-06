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

Merging a PR SHALL first bring its branch up to the current base by rebasing the
branch onto base, then fast-forward-free merge the branch into the base branch and
mark the PR merged. A PR SHALL only be merged after it is approved, and merging
SHALL be gated by the task's review gates. When the rebase hits conflicts — a
genuine divergence from base — the merge SHALL NOT proceed: the PR SHALL be routed
back to the owning worker to resolve and resubmit, and the conflict SHALL be
reported rather than silently swallowed.

#### Scenario: Gated merge

- **WHEN** a merge is attempted while the task has an unmet review gate
- **THEN** the merge is refused until the gate is satisfied

#### Scenario: Stale branch merges after auto-rebase

- **WHEN** an approved PR whose branch has fallen behind the base is merged
- **THEN** the hub rebases the branch onto the current base and, the rebase being
  clean, completes the merge with no human step

#### Scenario: Conflict returns to the worker

- **WHEN** rebasing the branch onto the current base conflicts
- **THEN** the merge stops, the PR returns to its owning worker with the conflict
  reported, and the worker resolves and resubmits

### Requirement: Role-scoped commands; merge is human-only

The agent client SHALL be a single role-agnostic browser whose available commands
are filtered by the hub from the caller's role and state. A worker's surface SHALL
expose registering and inspecting merge-intents but never approve/reject/merge; a
reviewer's surface SHALL expose approve/reject but never submit; a coauthor's
surface SHALL expose only the generic helpers (status, log, lint, and the
read-only PR views) and none of the build or review verbs — a coauthor commits
with git directly rather than through a hub verb; a planner's
surface SHALL expose reading the backlog, proposing tasks, and shipping openspec
(`task`/`create-task`/`openspec`) but never the worker's `next`/`submit` nor the
reviewer's `approve`/`reject`. Approval SHALL NOT be the reviewer agent's
exclusive power: the host SHALL also expose a human approve (`sindri pr approve`),
the positive counterpart of the existing human reject, so a PR can reach
`approved` without a reviewer agent in the loop. A human approve SHALL mark the PR
approved and satisfy its review gates exactly as a reviewer approve does, and SHALL
apply only to an open PR (one awaiting a verdict). Merge SHALL be human-only,
exposed only on the host and requiring explicit confirmation; no agent surface
SHALL ever include merge.

#### Scenario: Reviewer approves, human merges

- **WHEN** the reviewer approves a PR
- **THEN** the hub marks it approved and its gates satisfied, but it is merged only
  later by a human on the host

#### Scenario: Human approves without a reviewer

- **WHEN** no reviewer agent has approved a PR and the user approves it on the host
- **THEN** the hub marks it approved and its gates satisfied, so the user can then
  merge it — a reviewer agent is not required to reach `approved`

#### Scenario: Approve only an open PR

- **WHEN** a human approve targets a PR that is not open (already approved, merged,
  or rejected)
- **THEN** the approve is refused and the PR's current status is reported, mirroring
  the reviewer approve's open-only guard

#### Scenario: Planner ships, not builds

- **WHEN** a planner queries its surface
- **THEN** it can read the backlog, propose tasks, and ship openspec, but it has no
  `next`/`submit`/`approve`/`reject`, and no merge

#### Scenario: No agent merge

- **WHEN** any agent queries its command surface
- **THEN** no merge command appears; only the host `sindri pr merge` can merge,
  after human confirmation
#### Scenario: Coauthor has only helpers

- **WHEN** a coauthor asks the hub what it can run
- **THEN** it is offered the generic helpers only — no `next`/`submit`, no
  `approve`/`reject`

### Requirement: Self-contained, no remote dependency

The PR/worktree/merge workflow SHALL function with no network and no GitHub
account; branches, PRs, review, and the human merge gate all live in the local
git repository and never contact a remote. This offline guarantee covers the
*core loop*. It does NOT preclude sindri from having *separate, optional* network
integrations layered beside it — specifically, the GitHub *issue source* (see the
`github-issues` capability), which reads open issues inbound and closes an issue
on merge. Such an integration SHALL be optional and SHALL degrade to absent when
the network or GitHub is unavailable, so its absence never breaks the offline
core: with no network, create, review, and merge of local PRs all still work, and
merge SHALL never block on a GitHub write-back.

#### Scenario: Offline

- **WHEN** sindri runs with no network
- **THEN** create, review, and merge of local PRs all still work

#### Scenario: Merge does not block on GitHub

- **WHEN** a `gh-*` task's local PR is merged while GitHub is unreachable
- **THEN** the merge completes locally and the GitHub close/comment is skipped with
  a warning — the local merge is never blocked on the remote

#### Scenario: Issue source absent, core unaffected

- **WHEN** the GitHub issue source is disabled or unavailable
- **THEN** the local PR/worktree/merge workflow is unchanged and fully functional

### Requirement: td reads are direct, writes go through the tool

For the td backend, the td adapter SHALL read tasks directly from td's own SQLite
database for speed, but SHALL perform every write action (create, start, comment,
review, …) only through the `td` tool — never by writing td's database directly.
Both strategies SHALL be encapsulated in `internal/adapter/tasks/td` so internal logic
sees a single adapter interface.

#### Scenario: Fast read

- **WHEN** the hub syncs td tasks into its cache
- **THEN** it reads td's SQLite directly rather than invoking the td CLI per query

#### Scenario: Write through the tool

- **WHEN** a td task is created or mutated
- **THEN** the change goes through the `td` tool, never a direct write to td's DB

### Requirement: Container branches persist across subtasks

A held container's worktree branch SHALL be named for the container and persist
across all its subtasks, decoupled from the agent's current subtask: completing
one subtask and starting the next SHALL land both as commits on the one container
branch and SHALL NOT create or rename a branch. (This is the collaborative
exception to one-branch-per-leaf; a structured leaf task still gets its own
branch.)

#### Scenario: One branch, many subtasks

- **WHEN** an agent completes a subtask and moves to the next within the same
  container
- **THEN** both land as commits on the single container branch, which is neither
  recreated nor renamed between them

#### Scenario: Branch outlives the current subtask

- **WHEN** the agent's current subtask changes
- **THEN** the branch name does not, because it tracks the container, not the
  subtask

### Requirement: Milestone merge does not retire the branch

A container branch MAY be merged at a milestone: the merge SHALL land the branch's
current state into the base, rebase the branch onto the new base, and leave the
branch in place for continued work — distinct from a terminal merge, which retires
the branch and frees the agent. A milestone merge stays human-only and gated on an
approved PR (the human may approve it directly). The branch is retired only when
its container is closed.

#### Scenario: Milestone merge keeps the branch

- **WHEN** a container branch is merged at a milestone
- **THEN** its current state lands on base, the branch is rebased onto the new
  base, and it remains checked out for the agent to keep working

#### Scenario: Terminal merge on container completion

- **WHEN** a container is closed (all its children done) and its branch is merged
- **THEN** the branch is retired and the agent is freed to take new work

### Requirement: Planner ships openspec changes as a PR

A planner SHALL turn its openspec edits into a merge-intent with `openspec submit`,
reviewed and merged through the same cycle as a worker's PR. The planner SHALL work
on a standing branch (`plan-<name>`) rather than a per-task branch, and its PR SHALL
carry no real backlog task (a placeholder task id stands in for it). Submitting
SHALL run the same lint gate as a worker's submit — including openspec validation —
and refuse the PR if a gate fails. On reviewer rejection the planner SHALL drop to
idle with the feedback injected; after any merge moves the base branch, every
planner's standing branch SHALL be rebased onto the new base so planners stay
current.

#### Scenario: Shipping a plan

- **WHEN** a planner runs `openspec submit` with openspec edits that pass the gate
- **THEN** its standing branch is committed and a merge-intent is registered,
  reviewed like a worker's PR, with no backlog task behind it

#### Scenario: Plan fails the gate

- **WHEN** a planner submits openspec that fails the lint gate (e.g. invalid spec)
- **THEN** no PR is created and the violations are reported for the planner to fix

#### Scenario: Planner rebased after a merge

- **WHEN** a PR merges and moves the base branch
- **THEN** each planner's standing branch is rebased onto the new base so it sees
  the latest code

## Structure

- `internal/hub/repo/` (`type: logic`) — the git mechanics behind a PR:
  materializing a branch for review, scrapping one, and running the submit gate.
- `internal/hub/workflow/` (`type: logic`) — the PR lifecycle: submit, contribute,
  review verdicts, merge, and the directive each role is given.
- `internal/hub/store/` (`type: logic`) — the PR records, task cache and event log,
  in the hub's SQLite database.
- `internal/hub/commands/` (`type: logic`) — the role-filtered command surface an
  agent sees; `cmd/sindri-worker/` is the thin browser that renders it.

