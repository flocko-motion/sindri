# Route the submit/contribute quality gate through the run queue

## Why

`CmdSubmit` and `CmdContribute` ran the project's quality gates inline, as a host subprocess, once
per submitting agent, with no coordination between them. With a large fleet, several agents
submitting at once meant several full verify runs at once — the exact host contention the run
queue (this feature's sibling subtasks) exists to remove, except triggered automatically rather
than by an agent asking for a run. The feature caps the runs people ask for and leaves the ones
that fire automatically on every submit uncapped, which is the traffic that actually exhausts the
host.

## What changes

- The gate becomes a queued run, sharing the single fleet-wide slot, the same timeout, and the same
  stored-output-not-injected discipline as an agent-requested run.
- A gate run outranks an agent-requested (exploratory) run in the queue **by default**, regardless
  of priority code or arrival order: a gate blocks a PR, an exploratory run blocks nobody, and
  failing a gate on a timeout costs more than failing an ad-hoc one.
- `submit`/`contribute` return AT ONCE with the gate queued and its position, instead of blocking
  on the gate's result. The PR record is written only once the gate passes — the guarantee that a
  failing PR is never created holds exactly as it did when the gate ran inline. On failure, the
  agent is told the violations (the same rulebook a synchronous failure already showed) and no PR
  is created.
- A gate that times out, or is orphaned by a hub restart, is reported as **incomplete** — distinct
  from a lint violation. Neither found anything wrong with the code, and reporting either as a
  violation would send an agent "fixing" nothing that was ever broken.
- A new agent phase, `gating`, marks the wait between asking and knowing: there is no PR yet, so
  `submitted`'s wording (which claims one exists) would be false. Every place that treated
  `submitted` as "don't hand out new work, don't rebase this branch, don't say it's idle" now
  treats `gating` the same way.

## Scope

- `CmdSubmit`, `CmdContribute` — the two verbs the task names.
- NOT `CmdOpenspec` (a planner's `openspec submit`): its gate is openspec validation, materially
  cheaper than the code linter plus a declared verify command, and it never spawns a build or test
  run — the contention this proposal addresses.
- NOT `CheckOpenPRs` (the periodic re-check of an open PR against a moved base): it already
  self-serializes to at most one gate call per tick via its own mutex, a materially different (and
  already bounded) traffic pattern from N agents each firing an inline gate with no coordination.
- NOT `sindri lint` / `LintPR` (the explicit, human- or agent-invoked self-check and the reviewer's
  pre-verdict check): both are interactively invoked and expect an immediate synchronous answer,
  unlike submit/contribute where the caller already goes idle waiting for an injected result.

## Mechanically

The gate itself still runs via the existing host-subprocess gate directly against the agent's live
worktree — no container, no materialization. That is exactly what it already did inline; this
proposal only moves WHEN it runs, not HOW. A gate run is created through the same queue path an
exploratory run uses, tagged with which verb it is for and the agent's free-text summary (carried
across the wait so the eventual commit reads the same as an inline gate's would have).

## Impact

- Specs: the `03-gh-local` "Lint gate before submit" requirement — a change to the submit contract
  itself (queued, not inline; PR written later, not before returning), which other in-flight
  branches may build on.
- Code: `internal/hub/workflow/{pr,contribute,gate,run,execrun}.go`, the new `gating` phase and its
  call sites (`task.go`, `stall.go`, `commands.go`, `reference.go`), and the reply/message text in
  `submitreplies.go`/`injected.go`.
