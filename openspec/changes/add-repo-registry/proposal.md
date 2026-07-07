# Make repos first-class: a managed registry + a first-class TUI switcher

## Why

The hub is already multi-repo: it tracks a registry of known projects
(`repoTag → path`), and the Agents and PRs tabs already show the whole fleet
across every repo, each row tagged with its repo. But that registry is **invisible
and unmanaged** — a repo silently self-registers the first time the hub touches it,
and there is no way to *list*, *inspect*, *set up*, or *drop* a repo. Meanwhile the
Tasks tab is scoped to one repo (correctly — `td` is genuinely per-repo), and the
switch between repos is a quiet picker most users never notice.

That mismatch is what feels "strange": tasks are per-repo while agents/PRs are
global, and there's no first-class notion of "the repo I'm managing." The fix is
**not** to force tasks global (a merged `td` backlog would mislead). It's to make
the repo a real, managed, easily-switched entity — so per-repo tasks read as
intentional, and switching is frictionless and visible.

## What Changes

- **A `repo` command set** — `sindri repo init | list | info | forget`, on both the
  CLI and the TUI, to manage the registry the hub already keeps.
- **`repo init` is additive, not a gate.** It registers cwd's repo eagerly and
  scaffolds a committed `.sindri/config.yaml` (and seeds `ARCHITECTURE.md`) — a
  clean "this repo is set up" step. A repo you never `init` still self-registers on
  first use, exactly as today. Nothing is required; `init` is a convenience.
- **`repo forget` gives up management without deleting the repo.** It deletes the
  repo's agents (freeing their pods and worktrees) and removes the registry row, but
  leaves the repo itself alone — its `.sindri/` config and git history stay, and its
  passive hub records (cached tasks, priority overrides, approvals, PRs, event log)
  are retained keyed by the repo's stable tag. Re-adding the same repo reactivates
  those records; forgotten records don't surface in the global views meanwhile. (The
  name is deliberate — we forget the repo, we don't delete it.)
- **The repo switcher becomes first-class** in the TUI: a visible control, with the
  active repo labeled on the Tasks tab so per-repo scoping is obvious, not implicit.
- **Agents and PRs tabs gain a `global/repo` scope toggle** (default `global`). In
  `global` the tab shows the whole fleet across every repo (today's behavior); in
  `repo` it narrows to the active repo. Best of both — a fleet overview and a
  per-repo focus, chosen per tab.
- **Repo config editing in the TUI** — a form over the `.sindri/config.yaml` keys
  (`architecture`, `containerfile`, `review_prompt`, `github.issues`) so a repo can
  be configured without hand-editing YAML.

## Capabilities

### Modified Capabilities
- `project-registry`: the registry gains an explicit lifecycle — a repo MAY be
  registered up front (`repo init`, additive; implicit-on-first-use stays) and MAY
  be dropped (`repo forget`, registry-only, agent-guarded, files untouched); the
  registry is listable and inspectable through a first-class command set.
- `view-tui`: the repo switcher is a first-class, labeled control; the Agents and
  PRs tabs carry a `global/repo` scope toggle; a repo's `.sindri/config.yaml` is
  editable through a TUI form.

## Impact

- **CLI**: a new `repo` command group (`cmd/sindri/repo.go`), defaulting to cwd's
  repo like every other command.
- **Hub**: registry write surface — an explicit register (idempotent, already
  exists) and an `UnregisterProject` (agent-guarded); a `RepoInfo`/`RepoList`
  read for the command set; `repo init` scaffolds `.sindri/config.yaml`.
- **TUI**: promote the switcher; add the per-tab scope toggle (Agents, PRs); add a
  config-edit form bound to the config package's keys.
- **No task-model change.** Tasks stay per-repo — no `Project` field on
  `store.Task`, no global `AllTasks`, no regrouped task tree.

## Non-goals

- **Tasks do NOT go global.** The per-repo task scope is deliberate (`td` is
  per-repo); this change makes that legible via the registry + switcher, not by
  merging backlogs.
- **`repo init` is not a prerequisite.** Implicit registration on first use stays;
  `init` never becomes a required gate.
- **`forget` is not a permanent ban.** A forgotten repo re-registers on next use; a
  hard "ignore even if used" exclusion is out of scope.
- No change to the PR/merge workflow, agent lifecycle, or the wire protocol beyond
  the new registry read/write endpoints.
