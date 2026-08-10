# Tasks

## 1. Measure

- [x] 1.1 `adapter/agent.Agent` gains `ContextTokens(home string) (tokens int, ok bool)`;
      Claude Code's implementation reads the newest `*.jsonl` under
      `home/projects/*/` and sums the last assistant message's usage.
- [x] 1.2 `hub/agent.Service.ContextTokens` wraps it with a short TTL memo (the
      transcript grows with every turn, not every board read).
- [x] 1.3 `AgentView.ContextTokens` carries it to every front-end; the TUI detail pane
      and `agent info` show it next to memory.

## 2. Retire

- [x] 2.1 `workflow.Engine.contextFull` reads `deps.ContextTokens` against
      `ContextFullThreshold`; no recorded usage yet is never full.
- [x] 2.2 `claimNext` skips a full worker; both `AgentDirective` paths that would
      otherwise block in `waitForWork` return `DirFull` immediately instead.
- [x] 2.3 The board's `Status` reads `full` for a retired worker (`state.go`, next to
      the existing `stalled` override); the TUI colours it like `blocked`/`stalled`.

## 3. Clear, on confirmation

- [x] 3.1 `hub/agent.Service.ClearContext` refuses while the worker holds a task or a
      container; otherwise it sends `/clear` and re-injects `workflow.MsgKickoff`.
- [x] 3.2 Reachable via `sindri agent clear-context <name>` (HTTP route, client
      method, CLI command) — invoking it is the confirmation, no second prompt.

## 4. Verify

- [x] 4.1 `make verify` passes.
- [x] 4.2 `openspec validate --all` passes.
