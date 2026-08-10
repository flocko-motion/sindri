# Hub — delta

## MODIFIED Requirements

### Requirement: Abstract tasks are a cached read model

The hub SHALL hold abstract tasks in `hub.db` as a fast local read model, synced
from their sources of truth. Tasks MAY come from more than one source — sindri's own
store, openspec changes, and GitHub issues — merged into the one cache; each row's id
prefix records which source owns it: `sd-` a task sindri owns, `os-` an openspec change,
`gh-` a GitHub issue. `td-` is the recognised LEGACY form of sindri's own ownership,
minted before `sd-` and inherited from the tool its store replaced; such ids SHALL keep
working and SHALL NOT be rewritten, because a task id is embedded in its PR id, its branch
name and its agent's worktree path. Browsing reads — lists and the board — SHALL be served
from the cache. To bound staleness where it would mislead or cause a wrong decision, the
hub SHALL refresh from the source of truth: **all tasks at startup**; **a task immediately
before it is assigned** to an agent; and **a task immediately before its detail is shown**.
Periodic background sync and explicit user refresh MAY additionally run. A **network-backed
source** (e.g. GitHub issues) SHALL be throttled — served from a short-lived cache so the
frequent idle-worker resync does not exceed the remote's rate limits — and SHALL degrade to
contributing no tasks, rather than failing the read, when it is unreachable.

#### Scenario: The prefix names the owner

- **WHEN** a task id is read
- **THEN** `sd-` means sindri owns it, `os-` an openspec change, `gh-` a GitHub issue, and
  `td-` means sindri owns it too — the legacy form, still honoured

#### Scenario: A legacy id is never rewritten

- **WHEN** a task minted before `sd-` is closed, prioritised, commented on or merged
- **THEN** it is acted on under its existing `td-` id, which stays embedded in its PR id,
  its branch name and its worktree path
