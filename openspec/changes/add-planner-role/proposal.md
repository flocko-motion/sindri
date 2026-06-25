# Add the planner role

## Why

The specs describe two agent roles — worker and reviewer — but the implementation
now ships a third: the **planner**. The planner is the agent that shapes upcoming
work *with* the user: it reads the repo and specs, proposes backlog tasks, drafts
openspec, and ships those specs as a PR. It is live today (`hub.go` accepts
`--role planner`, `commands.go` gives it `task`/`create-task`/`openspec`,
`workflow_task.go`/`workflow_pr.go` implement its loop) but is undocumented, so
the specs no longer match reality.

This change reconciles the specs with the shipped planner. It is documentation of
existing behaviour, not new work — no code changes. While doing so it also
corrects two pre-existing cross-role drifts the planner threw into relief: the
hub does name an agent its own role (so "role invisible to the agent" was wrong),
and the dwarf-name pool is role-agnostic (so "the reviewer takes no dwarf name"
was wrong).

## What Changes

- **A third role.** Valid roles become `worker | reviewer | planner`. The planner
  is registered and launched like any agent, but its job is to plan, never to
  build.
- **Different mounts.** Unlike a worker/reviewer (full read-write workspace), a
  planner sees the whole repo **read-only** with `openspec/` overlaid
  **read-write** — it plans (specs + tasks) without touching code.
- **Its own command surface.** The planner's verbs are `task` (read the backlog),
  `create-task` (propose a task), and `openspec submit` (ship its specs as a PR).
  It has neither `next` nor `submit` (worker verbs) nor `approve`/`reject`
  (reviewer verbs).
- **Task proposals are user-gated.** `create-task` creates a td task flagged
  *pending* approval; no worker can claim it until the **user** approves it
  (`sindri task approve`/`reject`). The verdict is injected into any running
  planner. A pending or rejected task stays hidden from workers.
- **Plans ship as PRs.** `openspec submit` commits the planner's standing branch
  (`plan-<name>`), runs the lint gate (including openspec validation), and
  registers a merge-intent reviewed and merged through the same cycle as a
  worker's PR — except there is no backlog task behind it (mock id `os-new`).
  On reviewer rejection the planner drops to idle; after any merge every planner's
  branch is rebased onto the new base so it stays current.
- **Never auto-assigned.** A planner is never handed a backlog task. Its resting
  directive is to orient (read README, the backlog, the specs) and wait for the
  user to steer it.

## Capabilities

### Modified Capabilities

- `hub`: the command surface is state-filtered across three roles, and the
  hub holds the user-approval gate on planner-proposed tasks.
- `agent-runtime`: the planner is a third role that plans and never builds — never
  auto-assigned work, resting in an orient-and-wait directive; and the hub briefs
  an agent with its own role while the roster and other agents stay hidden
  (correcting the stale "role invisible to the agent" claim).
- `04-workers`: mount topology is identical for workers and reviewers but differs
  for the planner (read-only repo + writable openspec overlay); and the dwarf-name
  pool is role-agnostic (correcting the stale "reviewer takes no dwarf name"
  claim).
- `05-workflow`: plan/build/review separation now names the planner agent that
  drafts specs and proposes tasks with the user; task proposals are user-gated.
- `03-gh-local`: role-scoped commands include the planner's surface, and the
  planner ships openspec changes as a PR on a standing branch.
- `view-tui`: the Tasks tab marks planner proposals under the approval gate and
  approves/rejects them; the new-agent picker offers the planner role.
- `view-workers`: each agent is rendered with its own role — worker, reviewer, or
  planner — and a planner is never shown as "working".

## Impact

- **Specs only.** Delta specs for the five capabilities above. No code changes —
  the planner is already implemented in `internal/hub`.
- **Source of truth:** `internal/hub/{hub,commands,prompts,workflow_task,workflow_pr}.go`
  and `internal/hub/store/workflow.go`.
