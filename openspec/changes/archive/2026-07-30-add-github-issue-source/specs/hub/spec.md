# Hub — delta

## MODIFIED Requirements

### Requirement: Abstract tasks are a cached read model

The hub SHALL hold abstract tasks in `hub.db` as a fast local read model, synced
from their sources of truth. Tasks MAY come from more than one source — the task
backend, openspec changes, and GitHub issues — merged into the one cache; each
row's id prefix (`td-`, `os-`, `gh-`) records which source owns it. Browsing reads
— lists and the board — SHALL be served from the cache. To bound staleness where
it would mislead or cause a wrong decision, the hub SHALL refresh from the source
of truth: **all tasks at startup**; **a task immediately before it is assigned**
to an agent; and **a task immediately before its detail is shown**. Periodic
background sync and explicit user refresh MAY additionally run. A **network-backed
source** (e.g. GitHub issues) SHALL be throttled — served from a short-lived cache
so the frequent idle-worker resync does not exceed the remote's rate limits — and
SHALL degrade to contributing no tasks when it is unavailable, without failing the
sync of the other sources. Every write SHALL go to the source of truth through
that source's tool, and the hub SHALL update the cache to reflect it.

#### Scenario: Browsing served from cache

- **WHEN** the board or a UI lists tasks
- **THEN** they are read from `hub.db`, not by querying the backend per query

#### Scenario: Refresh all at startup

- **WHEN** the hub starts
- **THEN** it refreshes every task from the sources of truth into `hub.db`

#### Scenario: Refresh before assignment

- **WHEN** a task is about to be assigned to an agent
- **THEN** the hub refreshes that task from the source of truth first, so an already
  changed or closed task is never handed out

#### Scenario: Refresh before detail

- **WHEN** a task's detail is shown
- **THEN** the hub refreshes that task from the source of truth before presenting it

#### Scenario: Write reaches the source of truth

- **WHEN** a task is created or changed
- **THEN** the change is written through the backend's tool and the cached copy is
  updated to match

#### Scenario: Network source is throttled

- **WHEN** many resyncs occur in quick succession (e.g. an idle worker polling
  every few seconds) with the GitHub source enabled
- **THEN** the GitHub listing is served from a short-lived cache rather than hitting
  the remote on every resync

#### Scenario: One source unavailable, others still sync

- **WHEN** the GitHub source is unavailable during a sync
- **THEN** td and openspec tasks still sync and the cache updates; the GitHub source
  simply contributes no tasks
