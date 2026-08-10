# Retire a worker whose context is full, and clear it at a task boundary

## Why

Nothing tells a worker, or the board, when an agent's context is nearly gone. The
board's only signal is the tmux pane, pattern-matched into working/blocked/idle — it
has never known how much of the model's context a session is carrying, so an agent
sitting on hundreds of thousands of tokens reads exactly like a fresh one. Measured
while writing this: one live worker was carrying ~507k tokens. It kept being handed
new leaf tasks the whole time.

The fix has three parts, in dependency order:

- **Measure.** Claude Code already writes an exact usage figure to disk on every
  turn — the session transcript's last assistant message carries
  `input_tokens + cache_read_input_tokens + cache_creation_input_tokens`, the size of
  what it's currently carrying. Reading that off disk is exact where pane-scraping is
  a guess.
- **Retire.** Past a threshold, a worker stops being handed new leaf or container work
  — `claimNext` simply skips it — and the board says why (`status: full`) rather than
  showing an agent that looks idle-and-ignored. This needs no confirmation: it
  destroys nothing, it just stops adding to what's already there.
- **Clear, on confirmation.** The remedy is Claude Code's own `/clear`, sent into the
  live session, followed by the same kickoff a relaunch gets. This is only safe at a
  leaf task boundary — an agent with no held task or container — because a held
  task's memory of the worktree's contents would otherwise silently go stale (the
  next `claimLeaf` resets the worktree on its OWN claim, not this one). A human
  confirms by invoking the command; there is no second prompt on top of that, the same
  convention `agent delete` already uses for its own irreversible action.

Compaction — folding a container worker's context down instead of discarding it, at a
milestone merge where the worktree is kept — is a different tool for a different
boundary and is explicitly out of scope here.

## What Changes

- A coding-agent backend reports its live session's context size, read from its own
  transcript (`adapter/agent.Agent.ContextTokens`); Claude Code's implementation reads
  the last assistant message's usage from the newest `*.jsonl` under its home.
- `AgentView` carries `contextTokens`; the board's `status` reads `full` once a worker
  is retired, alongside the existing `down`/`stalled`/etc.
- `workflow.Engine.claimNext` (and the two `AgentDirective` paths that block on it)
  skip a worker whose context is past `ContextFullThreshold` (170k tokens) — the
  worker is told directly (`DirFull`) rather than left hanging in the wait loop.
- A new hub capability, `ClearContext`, sends `/clear` into a worker's session and
  re-serves its kickoff; it refuses when the worker holds a task or a container.
  Reachable via `sindri agent clear-context <name>`.

## Impact

- **Source of truth:** `internal/adapter/agent/claude/context.go` (the transcript
  read), `internal/hub/workflow/task.go` (`contextFull`, `claimNext`, `DirFull`),
  `internal/hub/agent/{runtime,clearcontext}.go`, `internal/api/board.go`
  (`ContextTokens`), `internal/hub/state.go` (the `full` status).
- No change to how a task is claimed once a worker is under the threshold — the gate
  is additive, checked before the existing assignment logic runs.
