## 1. The move — `internal/hub/api/` (commit 1, no behaviour edit)

- [x] 1.1 Create `internal/hub/api/` as a grouping directory with no Go files at its root
- [x] 1.2 Move `hub/server/` → `api/serve/` (package `serve`, so it never shadows `net/http`
      in `api/frontend`)
- [x] 1.3 Move `hub/registry/` → `api/agents/registry/` and rewrite the import in all 24
      non-test files that take a `registry.Caller`
- [x] 1.4 Move `hub/agent/agentchan/` → `api/agents/channel/`
- [x] 1.5 Move `hub/flow/agent/verb/` → `api/agents/verb/` and swap the entry in
      `flowMayImport` (`internal/arch/flow_test.go:38`)
- [x] 1.6 Move `hub/server.go`, `server_mail.go`, `server_runs.go`, `server_streams.go` →
      `api/frontend/`, converting the `(h *Hub)` methods to functions over a hub interface
      the package declares
- [x] 1.7 Rename `internal/hub/commands/` → `internal/hub/sections/` and fold
      `internal/hub/sections.go`'s `PROpen` into it
- [x] 1.8 Move each surface's tests with its code, naming them after the file they exercise
      (`brokkr lint test-home` gates this)
- [x] 1.9 Green: `go build ./...`, the full suite, all 8 linters

## 2. The catalogue — one definition per verb (commit 2)

- [x] 2.1 Declare `verb.Def` in `api/agents/verb`: `Name`, `Summary`, `Usage`, `UsageFor`,
      `Roles` — no `Run` field
- [x] 2.2 Write all 32 `Def` values, folding in the help the registry carries today
      (`hub/commands.go`) and the constants `verbs.GitHelp`, `verbs.ScratchHelp`,
      `task.CreateTaskHelp`
- [x] 2.3 Keep `verb.All` as the 22 the state machine governs, derived from the catalogue
      rather than listed a second time
- [x] 2.4 Build `registry.Command` from `Def` plus its bound `Run`; delete the help fields
      duplicated at registration
- [x] 2.5 Point the directive's offer list (`flow/fleet/flowsay.go:102`) at `Def.Summary`
- [x] 2.6 Write the binding table in `api/agents/exec.go`: verb name → implementation
- [x] 2.7 Fail the build (or a guard) on a catalogue entry with no binding, and on a binding
      naming no catalogue entry
- [x] 2.8 Move the six verbs still implemented in `hub/commands.go` to their subjects:
      status, log, comment, staff, meeting, reopen-task
- [x] 2.9 Move `contribute` to `flow/pr` — it is three calls into `pr.Act` already
- [x] 2.10 Rename `flow/agent/verbs/` → `flow/agent/workspace/`, holding `git` and `scratch`;
      update `isActing` in `internal/arch/flow_test.go`
- [x] 2.11 Add the `brokkr lint` rule pinning each registered verb's help against its `Def`,
      registered in `cmd/brokkr/lint.go` as the ninth linter
- [x] 2.12 Prove the help lint: give `git` two different help strings, watch it fail, restore
- [x] 2.13 Green: `go build ./...`, the full suite, all linters

## 3. The cores — one operation, one implementation (commit 3)

- [x] 3.1 Extract the `approve` core in `flow/pr` — fetch, `PRApprovable`, status write,
      badge, PR log, notify — taking the caller's voice as `reject` does
- [x] 3.2 Rewrite `CmdApprove` over it, keeping the self-review guard, the planner branch,
      the activity-log line and `completeReview`
- [x] 3.3 Rewrite `ApprovePR` over it, passing `api.SenderUser`
- [x] 3.4 Extract the `run` core behind `ScheduleRun` and `ScheduleUserRun`, which meet at
      `PutRun` today
- [x] 3.5 Rename the route `GET /log` → `GET /activity` and update both callers; the agent's
      `log` verb keeps its name
- [x] 3.6 Add the surface-separation arch guard: nothing under `api/agents` imports
      `api/frontend`, and the reverse
- [x] 3.7 Add the one-core arch guard: every `CmdX` and every route handler resolves to an
      exported function in a subject package, with an allowlist carrying a reason per entry
      (the shape of `ownedStatusWriters`)
- [x] 3.8 Prove each guard: point a handler at its own implementation, watch it fail, restore
- [x] 3.9 Move the approve and run tests to the packages holding their cores
- [x] 3.10 Green: `go build ./...`, the full suite, all linters

## 4. Close out

- [x] 4.1 Update the four stale `// package:` headers left by the earlier flow move
      (`flow/agent/{act,cond,says,verb}` still name `hub/flow/...`)
- [x] 4.2 Fix the mangled comment at `internal/adapter/agent/agent.go:67`, where
      `ModelWindow`'s doc is glued to `ContextUsage`'s signature line
- [x] 4.3 Consider a `brokkr lint` rule checking each `// package:` header against the file's
      real path, so a move cannot leave `brokkr map` describing the old tree
- [x] 4.4 Settle the design's open questions: where `pidfile.go` belongs, and whether
      `api/agents/channel` splits by transport
