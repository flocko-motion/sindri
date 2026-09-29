## Context

The hub answers two clients that share almost nothing. `cmd/sindri-worker` is a thin client
with no command tree: it dials the agent's own channel and either asks for a directive or
posts a verb with its argv. `cmd/sindri` and the TUI drive 63 REST routes on the control
socket. Those contracts differ in every dimension that matters — an agent verb is
`(Caller, []string, io.Writer) → (int, error)` returning prose for a model to read and gated
on where the flow machine has that agent; a route is a typed request in and JSON out, gated
on nothing.

The split is real and already load-bearing. What it lacks is a place in the tree, so the
pieces of each surface sit wherever they were first written:

| piece | today |
| --- | --- |
| the agent's verb list | `hub/commands.go` (`registry()`, 32 entries) |
| the agent's verb identities | `hub/flow/agent/verb/verb.go` (22 entries) |
| the agent's mechanism | `hub/registry/` |
| the agent's transport | `hub/agent/agentchan/` |
| three agent verb bodies | `hub/flow/agent/verbs/` |
| six more agent verb bodies | `hub/commands.go` itself |
| the front-end's routes | `hub/server.go`, `server_mail.go`, `server_runs.go`, `server_streams.go` — package `hub` |
| the shared HTTP conventions | `hub/server/` |

Two collisions follow from that: `verb` and `verbs` are sibling packages one letter apart
holding different kinds of thing, and `hub/commands.go` (agent verbs) sits beside
`hub/commands/` (dashboard tabs). A third is `hub/sections.go` holding one PR predicate while
`hub/commands/sections.go` holds the sections.

Seventeen of the 32 verbs have a front-end counterpart. The repo answers that overlap three
ways: a shared core (`reject` via `a.reject(project, prID, feedback, voice)`, `resume` via
`Resume`, `create-task` via `CreateTask`), a fork (`approve`: `CmdApprove` and `ApprovePR`
are two full implementations sharing one predicate; `run`: `ScheduleRun` and
`ScheduleUserRun` converge only at `PutRun`), and one name over two operations (`log` writes
for an agent, reads for a front-end).

## Goals / Non-Goals

**Goals:**

- One directory per surface, with the shared request conventions beside them.
- One definition per verb, carrying every word the agent reads about it.
- One core per operation, with a thin adapter per surface over it.
- Guards that hold all three, in the style `internal/arch` already uses.

**Non-Goals:**

- Merging the surfaces, or giving the front-end a verb registry. The contracts differ; the
  work here is to separate them properly.
- Changing what an agent sees. Verb names, help, roles and gating are carried through
  unaltered, and `verb.All`'s 22 identities keep governing the same states.
- Fixing the reviewer row a user verdict leaves open. It behaves the same on both the approve
  and the reject path, predates this change, and wants deciding on its own.
- Reworking the 63 routes. They move packages and one is renamed; their shapes stay.

## Decisions

### `internal/hub/api/` is a grouping directory, not a package

```
internal/hub/api/
    serve/        Decode, WriteJSON, LogRequests, accesslog   ← hub/server/
    agents/       the agent surface
        verb/         the catalogue: Def values, data only    ← hub/flow/agent/verb/
        registry/     Command, Caller, Standing, filtering    ← hub/registry/
        channel/      per-agent socket + TCP transport        ← hub/agent/agentchan/
        exec.go       the binding table and AgentExec         ← hub/commands.go
    frontend/     the routes                                  ← hub/server{,_mail,_runs,_streams}.go
```

`internal/hub/messaging/` is the precedent: it holds `chat/` and `mail/` and declares nothing
itself. A directory with no Go files at its root means no package is named `api`, so the
exchange package `internal/api` keeps the name unshared.

**`serve`, not `http`.** `api/frontend` imports `net/http`, so a package named `http` beside
it would need an alias at every such file. `serve.Decode`, `serve.WriteJSON` and
`serve.LogRequests` read as what they do and collide with nothing.

**`agents` plural, deliberately.** `hub/agent` (pod lifecycle) and `hub/flow/agent` (state
maps) already exist. A third package named `agent` would need an alias wherever two of them
meet; `agents` reads as "the agents' API" and needs none.

**`registry` goes inside `agents/`, not beside it.** It is agent-only: 24 non-test files
import it and none is a front-end. Its `Caller` is the agent-verb calling convention, which
is why `flow/pr/*_act.go`, `flow/task/*_act.go`, `messaging/mail/*` and `messaging/chat` all
take one. As a sibling of `agents/` and `frontend/` it would claim a sharing that does not
exist.

*Alternative considered:* `hub/verb` and `hub/registry` as flat siblings under `hub/`. It
solves the verb/verbs collision and nothing else — the 63 routes stay in package `hub`, which
is the larger half of the problem.

### `Def` is data, and never a `registry.Command`

```go
type Def struct {
    Name     string
    Summary  string                        // the line a state's offer list prints
    Usage    string                        // the full help; for git, the allowlist block
    UsageFor func(registry.Caller) string  // tailored where args differ by role
    Roles    []string
}
```

`registry.Command` carries a `Run func(...)`. Putting that in the catalogue would hand the
flow tree's declarations a field that can name a writer, which is the exact seam
`internal/arch/flow_test.go` exists to close — its allowlist applies to every non-acting file
under `flow/`, and the guard's value is that a state map provably cannot reach one. So `Def`
stays a plain struct and the binding lives in `api/agents/exec.go`:

```go
var bound = map[string]runner{
    verb.Submit.Name: func(h *Hub) runFn { return h.prFlow().CmdSubmit },
    ...
}
```

The catalogue then feeds all three readers that disagree today: the state maps
(`verb.Submit`), the registry (`Command` built from `Def` plus its bound `Run`), and the
directive's offer list (`Def.Summary`, today `flow/fleet/flowsay.go:102`). The `git` help
divergence — `verb.Git.Help` in the directive against `verbs.GitHelp` everywhere else — stops
being possible.

*Alternative considered:* one struct per command with a surface flag (`agent`, `frontend`,
`both`). Rejected on four counts. The signatures differ, so `both` needs two run fields and
the flag buys nothing two lists do not. The overlap is at the operation and not the command:
`CmdCreateTask` parses `--parent`, applies the priority-after-approval rule, logs and prints
a nudge, while `POST /tasks` calls `CreateTask` and returns an id. The cardinality does not
match — `approve` is one verb against two routes, `next` is a claim for the agent and an
explain-only read for the front-end. And the gating is asymmetric: `Roles` and `Blocked` are
permanently nil on the front-end side.

### One operation, one core, parameterised by who is asking

`reject` is the model already in the tree. `a.reject(project, prID, feedback, voice)` holds
the operation; `voice` decides the badge (`voice != "reviewer"` writes an `AddVerdict` row),
the message the author receives, and what the log records. `CmdReject` adds the self-review
guard and the reviewer's row completion; `RejectPR` passes `api.SenderUser` and adds nothing.

`approve` gets the same shape: a core doing the fetch, the `PRApprovable` check, the status
write, the badge, the PR log and the notify; `CmdApprove` keeps the self-review guard, the
planner branch, the activity-log line and `completeReview`. `run` gets one core behind
`ScheduleRun` and `ScheduleUserRun`, which already meet at `PutRun`.

Naming convention, since today's is `reject`/`RejectPR`/`Resume`/`ResumeByUser`/`CreateTask`/
`ScheduleRun`/`ScheduleUserRun` with no pattern: **the core takes the operation's name and a
caller parameter**; adapters take `Cmd` for the verb and the route's own handler.

### Two guards, in the style already used

`internal/arch/ownedstatus_test.go` walks the whole tree and names the three files allowed to
write one column. The same shape covers both new rules:

- **surface separation**: no file under `api/agents` imports `api/frontend`, and the reverse.
- **one core**: every `CmdX` and every route handler resolves to an exported function in a
  subject package, with an allowlist for the verbs that have no front-end counterpart.

Plus a `brokkr lint` rule pinning each registered verb's help against its `Def` — cheaper than
a test because it can point at the line.

### `GET /log` becomes `GET /activity`

The rename goes on the route rather than the verb. The agent's `log` is in prompts, in the
directive and in every agent's habit; the route is internal HTTP with two callers.

## Risks / Trade-offs

- **A 24-file import churn on `registry`, in one commit with a package split.** → Do it as
  step 1, a pure `git mv` plus import rewrite, with no behaviour edit in the same commit. The
  full suite and all 8 linters gate it.
- **`api/agents/registry` is five levels deep.** → `flow/agent/roles/worker` already is, and
  the depth buys the surface separation the tree is being reshaped for.
- **Converging `approve` changes what a user approve records** — it gains the activity-log
  line and the agent path's ordering. → This is the point of the step; it is called out in
  the proposal, and the existing approve tests move to `flow/pr` with the core.
- **The one-core guard could refuse a legitimate one-sided verb.** → It carries an allowlist
  with a reason per entry, as `ownedStatusWriters` does; a verb with no front-end counterpart
  names itself there.
- **Three steps in one change is a lot of tree movement at once.** → They are ordered so the
  risky one is last: two pure moves, then the behaviour convergence. Each step lands green on
  its own.

## Migration Plan

Three commits, in order, each green on the full suite and `brokkr lint`:

1. **The move.** `internal/hub/api/{http,agents,frontend}` with the pieces relocated,
   `flowMayImport` updated, `hub/commands/` → `hub/sections/`, `hub/sections.go` folded in.
   No behaviour edit.
2. **The catalogue.** `verb.Def`, the binding table, the six stragglers out of
   `hub/commands.go` into their subjects, `contribute` to `flow/pr`, `flow/agent/verbs/` →
   `flow/agent/workspace/`. The help lint lands with it.
3. **The cores.** `approve` and `run` onto shared cores, `GET /log` → `GET /activity`, the two
   arch guards.

Rollback is per commit: 1 and 2 are reversible moves, and 3 touches two operations.

## Settled Questions

- **`pidfile.go` stays in `api/serve/`.** It is 79 lines about the socket's owning process, read
  only by the hub entrypoint that binds that socket. Moving it to `internal/paths` would put
  process lifecycle in a path helper; a package of its own for one file buys nothing.
- **`api/agents/channel` stays one package.** 215 lines plus an 84-line darwin path, serving one
  surface over two transports because a bind-mounted unix socket cannot cross the podman VM
  boundary. Splitting by transport would put one contract in two packages and gain a boundary
  nothing needs.

## Discovered While Implementing

- **The maps offered two verbs that did not exist.** Four role maps offered `verb.Chat` (name
  `chat`) and the planner offered `verb.Plan` (name `plan`), while the registry served `meeting`
  and `openspec`. Every agent's directive listed two verbs that answered "unknown". Fixed by the
  catalogue, and held by `TestEveryVerbIsBoundBothWays` plus the `verb-help` linter.
- **`run` was not a fork.** `ScheduleRun` and `ScheduleUserRun` queue genuinely different things —
  an agent's run snapshots the workspace and task it holds; a user's names its target and executes
  against a copy. The only duplication was the empty-command check, now one `errNoCommand`.
- **Converging `approve` surfaced a double badge.** With the core writing the verdict for a
  non-reviewer voice, `stampVerdict` wrote a coauthor's a second time. `CmdApprove` now mirrors
  `CmdReject` exactly — `completeReview` for a non-coauthor, the core's badge otherwise — and
  `stampVerdict` is gone.
- **`hub/sections.go` was deleted rather than folded.** It held one alias, `var PROpen = api.PROpen`,
  with a single caller; that caller now says `api.PROpen` like the other nine.
- **`server_streams.go` stayed in package `hub`.** It is written over the hub's own change bus,
  which would have had to be exported to cross the package line. Its two handlers reach the routes
  through the `frontend.Hub` interface instead.
- **Two verbs stayed in `commands.go`.** `comment` and `reopen-task` each compose two subsystems the
  hub owns (comments plus the reviewer's task scope; the task flow plus comments), so the binding
  file is also their home, named and justified rather than silently excepted.
