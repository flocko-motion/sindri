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

Submitting (and contributing) SHALL run the project's quality gates through the same
run queue as an agent-requested run — the built-in gates (file length, dead code, and
OpenSpec validation) together with the project's own declared verify command (see
`project-config`), rather than inline. `submit`/`contribute` SHALL return at once with
the gate queued; the PR record SHALL be written only once it passes, so a failing PR is
never created. If any violation is found, or a gate cannot run (e.g. the code does not
compile), no PR SHALL be created and the violations SHALL be reported once the queued
gate completes. OpenSpec validation SHALL be skipped when the project doesn't use
openspec.

A gate run SHALL outrank an agent-requested run in the queue by default — it blocks a
PR where an exploratory run blocks nobody — and at most one gate (or exploratory run)
SHALL execute at a time across the whole fleet, so several agents submitting at once
costs one verify run at a time, not several concurrent ones.

A project that declares a verify command SHALL have it run whatever the project's
language: the absence of a Go module SHALL NOT be treated as the absence of a gate. A
project that declares none SHALL be gated by the built-in checks alone, as before.

The gate SHALL be bounded — a timeout, and output capped in a way that names what was
cut and how much, since silent truncation reads as a complete answer. Its output SHALL
be retained on the PR record and surfaced to the human, so a refused submit explains
itself without the check being re-run. A gate that does not complete within its
timeout SHALL be reported as incomplete, distinct from a lint violation: neither a
timeout nor a hub restart mid-gate found anything wrong with the code, and reporting
either as a violation would send an agent "fixing" nothing.

#### Scenario: Clean submit

- **WHEN** an agent submits work that passes every gate
- **THEN** the agent is told the gate is queued, and the PR record is created once the
  queued gate passes

#### Scenario: Lint violation

- **WHEN** an agent submits work that fails a gate (lint or an invalid spec)
- **THEN** no PR is created and the violations are shown for the agent to fix once the
  queued gate completes

#### Scenario: The project's own gate refuses a submit

- **WHEN** a project declares a verify command and it exits non-zero for an agent's
  work
- **THEN** the submit is refused, no PR is created, and the command's output is
  reported to the agent

#### Scenario: A declared gate is not skipped for a non-Go project

- **WHEN** a project with no Go module declares a verify command and an agent submits
- **THEN** the command runs and its result decides the submit, rather than the gate
  passing because no Go module was found

#### Scenario: A refused submit explains itself later

- **WHEN** a human inspects a PR whose gate failed
- **THEN** the retained gate output is shown, without the gate being run again

#### Scenario: A hanging gate does not hang the submit

- **WHEN** a project's verify command does not finish within the gate's timeout
- **THEN** the gate is reported as incomplete rather than as a lint failure, no PR is
  created, and the agent may submit again

#### Scenario: A gate outranks an exploratory run

- **WHEN** an agent-requested run is already queued and a submit's gate is queued
  behind it
- **THEN** the gate runs first, regardless of the exploratory run's priority or how
  long it has waited

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

### Requirement: td is a one-way import, not a backend

Sindri SHALL own its tasks in the hub's own store, and SHALL NOT depend on the td tool at
runtime. Where a repository carries an existing td database, the hub SHALL import that
backlog **once**, the first time it syncs the project, so adopting sindri costs nobody their
tasks. That import SHALL read td's own SQLite directly, and SHALL be the only way td is
touched: the td CLI SHALL NOT be invoked, and td's database SHALL NOT be written.

Because the import happens once, a task that arrives this way SHALL thereafter be an
ordinary task sindri owns, indistinguishable from one created in sindri — including its
`td-` prefix, which records sindri's ownership rather than a live backend (see `hub`).

#### Scenario: Existing backlog imported once

- **WHEN** the hub syncs a project whose repository has a td database for the first time
- **THEN** that backlog is imported into the hub's own store, and subsequent syncs import
  nothing further

#### Scenario: td is never written

- **WHEN** a task imported from td is created, changed, or closed
- **THEN** the change lands in the hub's own store, and neither td's database nor the td CLI
  is touched

#### Scenario: No td, no problem

- **WHEN** a repository has never used td
- **THEN** nothing is imported and the hub's own tasks are the whole backlog

## Structure

- `internal/hub/repo/` (`type: logic`) — the git mechanics behind a PR:
  materializing a branch for review, scrapping one, and running the submit gate.
- `internal/hub/workflow/` (`type: logic`) — the PR lifecycle: submit, contribute,
  review verdicts, merge, and the directive each role is given.
- `internal/hub/store/` (`type: logic`) — the PR records, task cache and event log,
  in the hub's SQLite database.
- `internal/hub/commands/` (`type: logic`) — the role-filtered command surface an
  agent sees; `cmd/sindri-worker/` is the thin browser that renders it.

