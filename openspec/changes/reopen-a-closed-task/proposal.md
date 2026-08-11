# Let the planner (and the host) reopen a closed task, with a reason

## Why

A planner cannot reopen a task. When a closed task turns out not to be done, the
only recourse is proposing a fresh task that cites the old one — which costs a
new approval and a new priority decision, and splits one defect across two rows.
It has happened twice: `td-5bb427` closed without making either of its two
`ARCHITECTURE.md` edits (continued as `td-eb3770`), and `sd-ac8831` closed having
advertised J/K in the footer without making them scroll (continued as
`sd-76552b`). Both times the mechanical half landed and the half needing
diagnosis in the running UI did not.

The operation already existed: `internal/hub/agent/lifecycle.go`'s
`DeleteAgent` reopens a task when its agent is deleted
(`ps.SetOwnedStatus(st.Task, "open")`), so releasing a sindri-owned task back
to open is not new machinery — it was simply never exposed as something a
planner or a human could ask for directly, on a task that had genuinely
closed rather than merely lost its agent.

`05-workflow`'s "The task lifecycle" requirement enumerates the states a task
travels and currently ends at "merged (task closed)" — after this change that
enumeration is no longer the whole lifecycle, and reopening carries rules
(scope, a required reason, who may invoke it) that belong on the record rather
than only in code comments and a CLI usage string.

## What Changes

- `workflow.Engine.ReopenTask(project, id)`: restores a closed task **sindri
  owns** (`sd-`/`td-`) to open. Refused for a `gh-`/`os-` id — its status comes
  from its own source (an issue tracker, an archived openspec change), not
  sindri's store — and for a task that is not currently closed. Goes through
  `SetStatus`, never `store.SetOwnedStatus` directly, so the read-model cache
  agrees with the owned row.
- **No new approval gate.** Reopening restores the release a closed task
  already had, not new work — it is not a second approve. The task's priority
  is left exactly as it stood, which alone can make it immediately claimable
  by a worker the moment it reopens.
- **A reason is required**, and is recorded as a comment on the task through
  the existing comment thread (`comments.Add`) — the "this did not hold"
  signal a fresh duplicate task would otherwise lose. `hub.ReopenTask` is the
  one funnel both callers below go through, so the reason is required and
  recorded exactly once.
- Exposed to:
  - the **planner**, via a new agent-facing `reopen-task <id> <reason...>` verb.
  - the **host**, in both front-ends: `sindri task reopen <id> <reason...>`
    (CLI) and `O` on a closed row on the Tasks tab (TUI).
  - **never a worker** — reopening a task it holds no relationship to is a
    human's or a planner's verdict to make, not an agent undoing one made about
    its own task.

## Impact

- **Source of truth:** `internal/hub/workflow/reopen.go` (the status flip and
  its refusals), `internal/hub/commands.go` (`Hub.ReopenTask`, the
  `reopen-task` verb), `internal/hub/server.go` (`POST /task/reopen`),
  `internal/client/client.go`, `internal/ui/cli/task.go`,
  `internal/ui/tui/tab_tasks_reopen.go`.
- No wire-shape change: `/task/reopen` reuses `RejectReq{ID, Feedback}`
  (`Feedback` carries the reopen reason).
- `openspec/specs/05-workflow/spec.md`: "The task lifecycle" gains the reopen
  path; a new requirement, "Reopening a closed task", specifies the scope,
  the required reason, and who may invoke it.
