# Add the coauthor role

## Why

The specs describe worker and reviewer roles (and the planner, in a sibling
change), each of which rides the same managed rails: the hub assigns work, the
agent acts in an isolated worktree, and a human gates the merge. That structure
is the point — it isolates agents and lets them touch the repo only through the
hub.

But there is a workflow the rails don't cover: the classic "just run Claude and
work with me" session, where a human and an agent edit the same code together,
freestyle, with no task and no review gate. This change adds a fourth role, the
**coauthor**, for exactly that — and it is live today (`hub.go` accepts
`--role coauthor`, shares the user's checkout, and shields `.sindri/`;
`prompts.go`/`workflow_task.go` give it a freestyle, non-blocking brief).

The coauthor is the deliberate exception to agent isolation: its `/workspace`
**is** the user's own working checkout, so the two edit the same material. The
hub still protects itself — `.sindri/` (hub.db, sockets) is hidden from the
shared tree — but otherwise the coauthor has full read-write access to the repo
and uses git directly, like a normal local Claude session.

This change also lands a runtime improvement that benefits every role: the user's
Claude **skills** are mounted into each agent's Claude home, so agents work with
the same skills the user has.

## What Changes

- **A fourth role.** Valid roles become `worker | reviewer | planner | coauthor`.
  A coauthor is registered and launched like any agent and auto-named from the
  same role-agnostic dwarf pool.
- **Shares the user's checkout.** Unlike every other role (an isolated per-agent
  worktree), a coauthor's `/workspace` is the user's repository checkout (the repo
  root) mounted **read-write** — it works the same material as the user. The
  `.sindri/` directory is overlaid by an empty **read-only** directory so the
  agent can neither read nor corrupt hub state in the shared tree.
- **Outside the managed loop.** A coauthor is never assigned a backlog task, never
  opens a managed PR, and never passes a review gate. The user steers it directly
  through its terminal; it edits files and uses git itself (the hub does not
  commit on its behalf as it does for a worker).
- **A minimal command surface.** A coauthor sees only the generic helper verbs
  (`status`, `log`, `lint`, and the read-only PR views) — none of the worker
  (`next`/`submit`/`checkpoint`), reviewer (`approve`/`reject`/`review`), or
  planner (`task`/`create-task`/`openspec`/`state`) verbs.
- **Never blocks.** A coauthor's next-action directive is a freestyle brief
  returned immediately; it never blocks on a work queue. Its resting status is
  `collab` (it stands with the user, so "idle" would mislead) and it is never
  shown "working" or "submitted".
- **User skills, mounted.** Every agent that runs Claude gets the user's
  `~/.claude/skills` mounted read-only into its Claude home — live (host edits
  show up without a relaunch) and read-only (the agent cannot alter them). A
  missing skills directory is fine; the launch proceeds without it.

## Capabilities

### Modified Capabilities

- `agent-runtime`: the coauthor is a fourth role driven directly (never
  auto-assigned, never blocking); and agent isolation gains its one exception —
  a coauthor's workspace IS the user's shared checkout, though `.sindri/` stays
  hidden even there.
- `04-workers`: mount topology gains the coauthor case (the user's repo root
  read-write, `.sindri/` shielded read-only); the dwarf-name pool is role-agnostic
  across all four roles; and every Claude agent mounts the user's skills read-only.
- `hub`: the state-filtered surface spans four roles; a coauthor sees only the
  generic helper verbs.
- `05-workflow`: the coauthor works outside the plan/build/review loop — no task,
  no managed PR, no review gate; it shares the checkout and uses git itself.
- `03-gh-local`: role-scoped commands include the coauthor's helper-only surface.
- `view-tui`: the new-agent picker offers the coauthor role.
- `view-workers`: each agent is rendered with its own role — including coauthor —
  and a coauthor's status is down/idle/collab, never working.

## Impact

- **Specs only.** Delta specs for the capabilities above. No code changes — the
  coauthor role is already implemented in `internal/hub`.
- **Source of truth:** `internal/hub/{hub,prompts,workflow_task,claude}.go`,
  `internal/tui/tab_agents.go`, and `cmd/sindri/hub.go`.
