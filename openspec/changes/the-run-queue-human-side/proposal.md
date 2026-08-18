# The run queue, from the human side

## Why

The run service shipped agent-facing. A user looking at the Runs tab sees work they cannot start,
and a human who wants a suite run has to ask an agent to queue it or run it outside the queue —
which is the uncoordinated concurrency the queue exists to prevent.

## What changes

- `sindri run new <command…>` queues a run as the user, and `N` on the Runs tab does the same. Both
  go through one client method into the same single slot agent runs use.
- The target is named, never inferred from the working directory: `--agent <name>` borrows that
  agent's workspace, and without it the target is the repo's own checkout. The TUI queues against
  the repo's checkout and says so in its prompt — that is the target only a human has; an agent's
  workspace is the agent's own to queue against, and the CLI covers it for the cases where a human
  wants to reproduce what an agent is seeing.
- A user's run goes to the FRONT of the queue, ahead of agent runs and their submit gates. Somebody
  is waiting on it; the agent behind a gate run is parked and watching nothing, and what the wait
  costs it is bounded by the 15-minute cap. `sindri run priority` still reorders it afterwards, so
  the origin decides where it starts, never that it is stuck there.
- `Run.Agent` carries `api.SenderUser` for a user's run — one "who asked" column rather than a
  second parallel field, using the sentinel chat and task comments already use for the human. Both
  front-ends render it as "you", since a column of agent names with the word `user` in it reads as
  another agent.
- Both front-ends now also name the workspace a run executes against. A run is uninterpretable
  without it: the same command passes in one tree and fails in another.

- The Runs tab carries a permanent line saying what a run is, that the queue is serial across every
  repo, and that both the user and agents queue them. Permanent rather than an empty state: the
  serial queue is the surprising part, and the question it prompts arrives when the list is full.
  Its lines come out of the rows' space, so it costs rows rather than pushing them off the screen,
  and the "(no runs)" empty state stays beneath it — the line says what runs are, not whether there
  are any.

## The target decision

Three candidates were on the table. Two are in:

**An agent's workspace** — already how agent runs work, and the reason a human wants it is to
reproduce what an agent is seeing without asking the agent to do it.

**The repo's own checkout** — allowed, and this is the one that needed deciding. It is safe because
`repo.MaterializeRun` copies the target into `.worktrees/run-<id>` and the container mounts the
COPY, so nothing a command writes reaches the tree the user is editing. That is the same isolation
sd-938f23 chose for agent worktrees, and it answers this case unchanged. Testing uncommitted work is
the point of the target rather than a hazard of it.

The copy has a second direction, though, and this target is what exposed it: with the repo ROOT as
the source, the destination sits inside the source. `.worktrees` was not skipped, so the copy took
every other agent's live worktree — uncommitted work included — and then descended into its own
half-built destination and copied that into itself. It joins `runSkipDirs`, which is right for both
targets: a run has no business copying any worktree but the one it was aimed at, and since every
destination lives under `.worktrees` it closes the self-nesting by construction.

**A PR's review checkout is OUT.** `.worktrees/review` is a single shared worktree that `pr verify`
and reviewer assignment both reuse. Materialised at schedule time, a queued run would execute
against a checkout that has since been replaced; materialised at execution time, it would fight a
live review for the slot. `pr verify` already answers "does this PR pass the gate", synchronously,
which is the question. Putting a PR in the queue needs the review worktree to stop being one slot —
its own piece of work, not a flag here.

## Impact

- Specs: `hub` gains the requirement for the human side of the run queue.
- Specs: `view-tui` gains the requirement for the tab's own explanation.
- Code: `internal/api/run.go` (`RunFromUser`, `RunTarget`), `requests.go`, `hub/workflow/run.go`
  (`ScheduleUserRun`, the queue rank), `execrun.go`, `hub/server_runs.go` (new — the run routes,
  lifted out because `server.go` had grown past its limit), `client`, `ui/cli/run.go`, `ui/tui`.
- `ExecuteRun` now materialises `r.Workspace` instead of re-reading the scheduling agent's. That is
  required for a user run, and it fixes a latent bug: the row already stored the workspace snapshot
  and the executor ignored it, so an agent that moved between queue and execution had its run
  redirected to wherever it had moved to.
