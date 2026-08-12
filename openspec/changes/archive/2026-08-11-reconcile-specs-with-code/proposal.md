# Reconcile the specs with the code they describe

## Why

Several specs describe a system that no longer exists. The most consequential is the
task model, which planners and workers reason about every day.

**Sindri owns its tasks now; the specs still say td does.** `internal/hub/store/owned.go`
holds an `owned_tasks` table, `internal/hub/workflow/ownedsource.go` presents it "through
the same Source interface openspec and GitHub implement", and
`internal/hub/workflow/importtd.go` carries a repo's existing td backlog into that table
"once, the first time the hub syncs that project — so taking td out costs nobody their
tasks". The td adapter never invokes the td CLI at all — it reads td's SQLite once, for
that migration. But:

- `hub`'s cached-read-model requirement names three sources — "the task backend, openspec
  changes, and GitHub issues" — and omits the one that is now primary, sindri's own.
- It says each id prefix "records which source owns it", with `td-` meaning the task
  backend. `OwnedPrefix = "td-"`: a `td-` id now means **sindri** owns the row, the prefix
  kept for continuity.
- It says "every write SHALL go to the source of truth through that source's tool", which
  is false for an owned task — the hub *is* the source of truth and writes its own table.
- `03-gh-local` still requires that the td adapter "SHALL perform every write action …
  only through the `td` tool", describing a live backend that is now a one-way import.

**Two smaller divergences.** `project-config` says the GitHub issue source defaults to
`false`; the `github-issues` capability says it is on by default (opt-out), the README says
so, and `config.IssuesEnabled` documents "defaults to ON" with `TestIssuesOptOut` pinning
it. The project-config requirement is the stale one. And two shipped integrations have no
spec at all: the per-agent container memory limit and the memory-versus-limit report
(`agent memory`, `agent stats`), and the herdr pane reporting that
`internal/adapter/herdr` and `internal/ui/attach/herdr.go` implement.

**Stale prose sections.** `01-architecture`'s Source layout, and the Structure sections of
`03-gh-local` and `04-workers`, name seven packages and binaries that do not exist:
`internal/issue`, `internal/board`, `internal/render`, `internal/ghlocal/store`,
`internal/worker`, `internal/agentcli`, `cmd/sindri-review`. The README's picture is stale
in the same way — it describes "a single per-repo hub" with state in `.sindri/hub.db` and
"Tasks (source of truth) | td", where the hub is global, its state is central under
`tools/paths.StateDir()`, and sindri owns the tasks. About twenty shipped commands are
missing from its reference.

Four completed changes are also unarchived (`add-repo-registry`, `add-coauthor-role`,
`add-planner-role`, `add-host-pr-approve`), so the base specs still carry text those
changes already corrected — `04-workers` still says the review agent "SHALL not take a
dwarf name", which the planner change fixed.

## What Changes

No behaviour changes. This change makes the specs describe the code.

- **The task model is restated**: sindri's own tasks are a source and the primary one, a
  `td-` prefix records sindri's ownership, an owned write goes to the hub's own store
  (which needs no refresh, being the source of truth), and a mirrored write goes through
  its source's tool.
- **td becomes what it is**: a one-way import at first sync, not a live backend that is
  written through.
- **The GitHub issue toggle default is corrected** to on-by-default, matching the
  `github-issues` capability and the code.
- **Two shipped integrations get specified**: per-agent memory limits with their
  observable usage, and the best-effort pane reporting to an external tracker.
- **The stale prose sections and the README are corrected** — after
  `separate-hub-from-frontends` lands, so the layout is described once and correctly
  rather than twice.
- **The four completed changes are archived**, which is what actually removes the
  superseded text from the base specs.

## Capabilities

### Modified Capabilities

- `hub`: abstract tasks are a cached read model — sindri's own tasks are the primary
  source, the id prefix records the owner, and the write rule distinguishes an owned task
  from a mirrored one.
- `03-gh-local`: td is a one-way import taken once at first sync, not a backend written
  through.
- `project-config`: the GitHub issue source is on by default, matching `github-issues`.

### Added Capabilities

- `agent-runtime`: an agent's container memory limit is declarable and its usage against
  that limit observable; attaching reports the occupied pane to an external tracker,
  best-effort.

## Impact

- Specs only, plus the README and the archiving of four finished changes. No source file
  changes, except that the sequencing matters: the layout corrections land after
  `separate-hub-from-frontends`, or they would describe the layout twice and be wrong once.
