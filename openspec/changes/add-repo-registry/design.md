# Design — a managed repo registry + first-class TUI switcher

## Context

The registry already exists: `store.Project{Tag, Path, FirstSeen}`, written by
`Store.RegisterProject(tag, path)` from `(*Hub).repo(root)` on first use, read by
`Store.Projects()` and surfaced as `BoardState.Projects`. The TUI already has a
switcher and already renders Agents/PRs globally with repo tags. What's missing is
a *management surface* (init/list/info/forget) and a *first-class* switcher + scope
toggle. This change adds those; it does not rebuild the registry or touch the task
model.

## The `repo` command set

CLI group `sindri repo …`, defaulting to cwd's git root like every other command:

```
sindri repo init            # register cwd + scaffold .sindri/config.yaml (additive)
sindri repo list            # every registered repo: tag, path, #agents, source flags
sindri repo info [repo]     # one repo: resolved config + counts (agents/PRs/tasks)
sindri repo forget <repo>   # drop the registry row only (agent-guarded, files kept)
```

`repo` (no subcommand) prints `info` for cwd's repo.

### init is additive

`init` does three things, none of which gate anything:
1. `RegisterProject(tag, root)` — idempotent; the same call implicit registration
   already makes, just eager.
2. Scaffold `<root>/.sindri/config.yaml` if absent — a commented template of the
   config package's keys. Never overwrites an existing file.
3. Seed `ARCHITECTURE.md` (reusing `ensureArchitectureDoc`) when the project hasn't
   configured its own `architecture` path.

A repo that is never `init`ed still self-registers on first use — `init` only
front-loads the setup and hands the user a config file to edit. No code path
requires a repo to be init'ed first.

### forget tears down agents, keeps the repo (soft records)

`forget` deletes the repo's agents (each via `DeleteAgent` — freeing pods,
worktrees, identities), then calls a new `Store.UnregisterProject(tag)` that removes
the registry row. It never touches `.sindri/`, the repo's git state, or the repo's
**passive hub records** — the cached tasks, `task_priority` overrides,
`task_approval` gates, PRs, and event log all stay in the store, keyed by the repo's
stable path-derived `repoTag`. Because that tag is a digest of the absolute path,
re-adding the same repo resolves to the same tag and every retained record re-links
automatically: that tag-stability *is* the soft-delete, so no per-row "deleted" flag
is needed. The verb is *forget*, not *delete* — a hard teardown of the running
agents, a soft forget of the repo's records.

To keep a forgotten repo out of the **global** views (PRs are read globally via
`AllPRs`), the board's PR list is scoped to registered projects: forgotten records
stay in the store but stop surfacing until the repo is re-added. Agents are deleted
outright, so `AllAgents` is already clean.

## TUI: switcher + scope toggle + config form

### First-class switcher — a persistent indicator + a scalable picker

Two parts, because "which repo am I in" and "switch repo" are different needs:

1. **Persistent current-repo indicator.** The active repo's name is always visible
   in the top bar (herdr/tmux-style — a fixed "you are here"), colored with that
   repo's deterministic scheme. It is not tucked inside the overlay; per-repo scoping
   is never silent. This is the load-bearing requirement — a user must be able to
   tell at a glance which repo the Tasks tab (and `repo`-mode tabs) reflect.

2. **A picker overlay to switch.** Opened with a key, it lists the registered repos.
   It is a **modal list, not a tab strip** — there can be many repos, so a
   fixed-width tab row won't scale; a scrollable, filterable overlay does. Ordering
   puts the most-relevant repos first:
   - repos with **live agents** on top (where work is actually happening),
   - then by **recency** (last-used / last-touched),
   - then the rest alphabetically.

   A typeahead filter narrows a long list. (Pinned "favorites" are a natural
   follow-up but out of scope for v1 — recency + active-first covers the common
   case without new state.)

Selecting a repo rescopes the Tasks tab and drives `repo`-mode Agents/PRs (below).
Recency ordering needs a "last active" signal per repo; the registry's `FirstSeen`
is insufficient, so this adds a `LastUsed` touch on the project row (updated on
register/use) — a small registry addition, not a task-model change.

### global/repo scope toggle (Agents, PRs)

Each of the Agents and PRs tabs gets a scope toggle (a key, e.g. `g`), cycling
`global ↔ repo`, defaulting to `global`:
- **global** — the whole fleet across every repo, repo-tagged (today's behavior).
- **repo** — filtered to the active repo (the switcher's selection).

This is a *view filter* over the already-global `BoardState.Agents`/`.PRs` — no new
hub call, no data-model change. The active mode is shown in the footer, like the
existing Tasks filter toggle. Tasks remain always-scoped to the active repo.

### Config editing

A form (reusing the task-form field primitives) over the `.sindri/config.yaml`
keys: `architecture`, `containerfile`, `review_prompt`, `github.issues`. It writes
the file through the hub (the writer), then the config package's normal load path
validates on next use — the form surfaces a validation error rather than persisting
a bad config. This is convenience over the file; hand-editing YAML stays valid.

## Why not global tasks

`td` is per-repo (each repo owns its `td` SQLite db); a merged cross-repo backlog
would present tasks from unrelated projects as one list and invite mis-assignment.
Agents and PRs are safe to globalize because each is unambiguously repo-tagged and
the fleet view is genuinely useful; a task tree is only meaningful within its repo.
So the registry + switcher — not a global task model — is the right unit of
multi-repo management. `store.Task` keeps no `Project` field and there is no global
`AllTasks`.

## Edge cases

- **forget a repo with live agents** — refused with a clear message; stop/delete the
  agents first.
- **forget then use again** — silently re-registers (documented, intended).
- **init an already-registered repo** — idempotent register; scaffolds only the
  missing files; never clobbers an existing `.sindri/config.yaml`.
- **info/switcher for a repo whose path moved** — the registry stores the absolute
  path at registration; a moved repo re-registers under a new tag on next use (path
  is the identity input to `repoTag`). `list` showing a stale path is a signal to
  `forget` the old row.
