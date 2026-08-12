# Design — the exchange package and the server binary

## Context

The hub is already a server: it owns all state, serves HTTP/JSON over a socket, and
every front-end talks to it through `internal/hub/client`. What is missing is the
*boundary*. `internal/hub/wiring.go` re-exports the core's sub-packages so the
front-ends can import the hub as their API, and `internal/ui/cli/hub.go:169` runs
the hub in-process for `sindri hub start`. So the client/server split is real at
runtime and absent at compile time.

This change makes the boundary structural in the two places that matter: a package
of exchange types that neither side owns, and a binary that neither side shares.

## What goes in `internal/api`, and what it may import

The inventory already exists — it is the alias block in `wiring.go` plus the store
row types the front-ends name. Concretely:

- **From `internal/hub`** (17 types): the views `AgentView`, `AgentStatsView`,
  `BoardState`, `ChatView`, `RepoDocState`, `StatsReport`, `CmdInfo`, and the
  requests `AgentReq`, `ChatSayReq`, `NameReq`, `PlanReq`, `PriorityReq`,
  `RejectReq`, `RepoReq`, `ScrapTaskReq`, `TaskReq`, `TellReq`.
- **The 8 aliased types**: `workflow.TaskSpec`, `workflow.PRDetail`,
  `project.Summary`, `project.Detail`, `agentchan.ExecReq`, `task.TaskRow`,
  `agent.ClientView`, and the section type.
- **From `internal/hub/store`** (8 of its 13): `Task`, `PR`, `Project`,
  `ChatMessage`, `ChatMember`, `Comment`, `Event`, `Review`. `AgentState`,
  `OwnedTask`, `Agent`, `Store` and `ProjectStore` stay — they never cross the wire.
- **The pure functions over them**: task arrangement (`ArrangeTasks`,
  `Descendants`), the open/done predicates, and the resolved section list.

**The package imports nothing internal, and nothing beyond the standard library.**
That is the invariant worth enforcing, because it is what lets both sides depend on
it without either dragging the other. Verified against the current code: the 17 hub
types reference only store row types, which move with them; `PRDetail`, `TaskRow`,
`TaskSpec`, `Summary`, `ClientView` and `ExecReq` are pure or store-only; and
`internal/hub/store` itself imports only `database/sql`, `fmt`, `strings`, `time`
and the sqlite driver.

## The two types that need a cut, not a move

**`project.Detail` embeds `config.Config`**, and `config.Config` crosses the wire in
its own right (the TUI's repo-config editor reads and writes it). Moving `Detail`
as-is would drag `internal/config`, and behind it `internal/tools/paths` and yaml.
The cut: `Config`, `GitHub` and `Lint` are pure data — strings, `*bool`, `*int`,
`*float64`, `[]string` — so the three structs move into `internal/api` and
`Load`/`Write`/`validate`/`Abs` stay in `internal/config`, importing them. One type,
no mapping layer, and the config editor is untouched. `internal/config` then imports
`internal/api`, which imports nothing, so there is no cycle.

**`commands.Section` carries `Count func(Board) int`** over an interface internal to
the hub. A function cannot serialise, so `Section` is not an exchange type today:
the count is computed hub-side against `BoardState` and only the number crosses the
wire. So the registry keeps its recipe and stays internal, and `internal/api`
carries the resolved `{Key, Title string; Count int}`. This is the one place where
the wire type differs from the internal one and a small mapping is unavoidable —
worth stating so nobody later "fixes" it by exporting the function.

## Why a separate binary rather than a lint rule

Separate binaries do not by themselves forbid the import — `cmd/sindri` could still
link `internal/hub` through `internal/ui/cli`. What the binary does is remove the
last legitimate *reason* to: `hub.New()` exists in the CLI only because the CLI runs
the daemon. Move that to `cmd/sindri-hub` and every remaining import is a mistake,
which is the precondition for enforcing it.

The mechanism is already in place. `ui/cli/hubclient.go:117-134` starts a detached
`sindri hub start` by re-execing the running binary, so auto-start is a process
spawn today and only its target changes. Foreground `sindri hub start` can
`syscall.Exec` into the hub binary so signals land on the hub directly with no
wrapper process; `--bg` keeps the detached spawn. The binary is resolved beside
`argv[0]` before falling back to `PATH`, the same rule `hub/agent/binaries.go`
already uses for the pod binaries and the reason `ui/cli/shadow.go` warns about a
second copy on `PATH`.

Nesting the hub under `cmd/sindri-hub/internal/hub` would let the compiler enforce
the boundary outright, since Go refuses an `internal/` import from outside its
parent tree. It is rejected here for two reasons: `cmd/` holds thin entrypoints by
rule (`entrypoint` — "wires a command tree and dispatches. No logic"), and burying
the core under one binary's directory would say the hub is a detail of that binary
rather than the centre of the system.

## Enforcement

A Go test under `internal/ui` walks the import graph and fails if any package there
imports `internal/hub/…`. It lives beside the code it constrains, it is the
project's own code rather than a rule added to a generic toolbelt, and `make verify`
already runs `go test ./...`, so it is checked on every build and by CI on every PR.

It is worth knowing what this does *not* cover: the agent's submit gate runs only
`brokkr lint` in the worktree (`internal/hub/repo/repo.go:44-56`, which also returns
a clean pass for any project without a `go.mod`), so a worker can still submit a PR
that breaks the rule and have it caught at review rather than at submit. Closing
that is a separate change (`stricter-submit-gate`), and this change does not depend
on it.

## Where presentation goes

`internal/ui/theme` is already imported by both front-ends and already exists to
keep them agreeing, so it is the UI-neutral rendering module the architecture spec
asks for. It receives: the priority and state display words, the client-list
formatting, and the chat glyphs and help text it currently re-exports *from*
`internal/hub/chat` — which inverts that dependency. The colour palette index stays
a stored per-machine preference, but the palette and the mapping to it are the
theme's.

The tree and section models are deliberately *not* sent to the theme. They are
functions over the exchange types that the hub needs too, so they belong in
`internal/api` — which is also what makes them one definition shared by every
front-end, the property the current spec was reaching for when it called the
arrangement "a logic-layer function".
