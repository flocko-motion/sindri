# Hub — delta

## MODIFIED Requirements

### Requirement: Abstract tasks are a cached read model

The hub SHALL hold abstract tasks in its store as one read model, drawn from more than one
source. **Sindri owns tasks of its own** — the primary source, held in the hub's own store
— and MAY additionally mirror tasks from external sources: openspec changes and GitHub
issues. Each row's id prefix records which source owns it: `sd-` a task sindri owns, `os-`
an openspec change, `gh-` a GitHub issue. `td-` is the recognised legacy form of sindri's
own ownership, minted before `sd-` and never rewritten (-> mint-sd-task-ids). Browsing
reads — lists and the board — SHALL be served from this model.

A write SHALL reach whatever owns the task. For a task sindri owns, the hub's own store is
the source of truth and the write lands there directly. For a mirrored task, the write
SHALL go through that source's tool and the cached copy SHALL be updated to match.

To bound staleness where it would mislead or cause a wrong decision, the hub SHALL refresh
**mirrored** tasks from their sources: **all at startup**; **one immediately before it is
assigned** to an agent; and **one immediately before its detail is shown**. A task sindri
owns needs no such refresh, being already authoritative. Periodic background sync and
explicit user refresh MAY additionally run. A **network-backed source** (e.g. GitHub
issues) SHALL be throttled — served from a short-lived cache so the frequent idle-worker
resync does not exceed the remote's rate limits — and SHALL degrade to contributing no
tasks when it is unavailable, without failing the sync of the other sources.

#### Scenario: Browsing served from the read model

- **WHEN** the board or a UI lists tasks
- **THEN** they are read from the hub's store, not by querying each source per query

#### Scenario: Refresh all at startup

- **WHEN** the hub starts
- **THEN** it refreshes every mirrored task from its source

#### Scenario: Refresh before assignment

- **WHEN** a mirrored task is about to be assigned to an agent
- **THEN** the hub refreshes it from its source first, so an already changed or closed task
  is never handed out

#### Scenario: Refresh before detail

- **WHEN** a mirrored task's detail is shown
- **THEN** the hub refreshes it from its source before presenting it

#### Scenario: An owned task is written directly

- **WHEN** a task sindri owns is created or changed
- **THEN** the hub writes its own store, which is the source of truth, with no external tool
  involved and no refresh needed

#### Scenario: A mirrored write reaches its source

- **WHEN** a mirrored task is changed
- **THEN** the change goes through that source's tool and the cached copy is updated to
  match

#### Scenario: The prefix names the owner

- **WHEN** a task id is read
- **THEN** `sd-` means sindri owns it, `os-` an openspec change, `gh-` a GitHub issue, and
  `td-` means sindri owns it too — the legacy form, still honoured

#### Scenario: Network source is throttled

- **WHEN** many resyncs occur in quick succession (e.g. an idle worker polling
  every few seconds) with the GitHub source enabled
- **THEN** the GitHub listing is served from a short-lived cache rather than hitting
  the remote on every resync

#### Scenario: One source unavailable, others still sync

- **WHEN** the GitHub source is unavailable during a sync
- **THEN** sindri's own tasks and openspec tasks are unaffected and the read model updates;
  the GitHub source simply contributes no tasks
