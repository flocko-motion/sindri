# Separate the hub from the front-ends: an exchange package and a server binary

## Why

The hub is meant to be the API the front-ends speak to — a server, with the CLI and
TUI as its clients. The specs say so twice: application logic is hosted by the hub
and "all user interfaces (CLI, TUI) and all agents SHALL be thin clients of that
hub", and a shared domain entity's model "SHALL live in its own package that both
the orchestrating service and every user interface depend on", with the explicit
prohibition that "a user interface SHALL NOT import the orchestrating service
package merely to reference a domain type".

The code does the opposite, deliberately. `internal/hub/wiring.go` states the intent
in its own words — "Module DTOs re-exported so hub stays the single facade its
clients import" — and re-exports the hub's sub-packages as the API its clients read:

- 24 files under `internal/ui` import `internal/hub` (`hub.BoardState` in 51 places,
  `hub.AgentView` in 46).
- 12 of them import `internal/hub/store` as well, so `store.Task` — a SQLite row
  type — is the task model the views render (39 uses), the equivalent of a web
  server publishing its ORM rows.
- `internal/hub/task` (the domain) imports `internal/hub/store` (persistence),
  which the architecture spec forbids by name.
- `internal/ui/cli/hub.go:169` calls `hub.New()` and runs the daemon in-process, so
  the front-end binary links the entire core, sqlite driver and every adapter
  included.

Two further consequences of the same muddle: presentation has leaked *into* the API
— `hub/chat/service.go:44-77` owns `HelpText` and the `👤`/`🤖`/`⚙` glyphs,
`hub/task/view.go` produces display words and a `Last` field that exists "for
drawing tree connectors", and `hub/project/project.go:195` validates a colour
palette index — while the front-ends reach *past* the API to do hub-side work
themselves: `ui/tui/startup.go:53` invokes the openspec adapter (probing the TUI's
PATH, not the hub's), `ui/tui/nested.go:39` imports `hub/server` for a liveness
check, `ui/cli/hub.go:183` calls `agent.SyncPodBin()`, and `ui/cli/hublist.go:76`
shells out to `ps` for hub uptime instead of reading it from the board.

## What Changes

- **The exchange format becomes its own package, `internal/api`, that imports
  nothing.** It holds the wire types and the pure functions over them: the hub's 17
  request/response types, the 8 types `wiring.go` aliases today, and the 8 store row
  types that travel on the wire. `internal/hub/store` then imports it rather than
  being what everyone imports. Two types need a cut rather than a move: the
  `Config`/`GitHub`/`Lint` structs move out of `internal/config` (they are pure data;
  `Load`/`Write`/validation stay behind and import them), and `commands.Section`
  keeps its `Count func(Board) int` registry hub-side while the wire carries the
  resolved `{Key, Title, Count}`.
- **The hub becomes its own binary, `cmd/sindri-hub`.** `sindri hub start` execs it
  instead of calling `hub.New()`; the existing detached spawn in
  `ui/cli/hubclient.go:117` already has this shape and only changes target. The
  front-end binary stops linking the core.
- **Nothing under `internal/ui` imports `internal/hub`.** The front-ends see the
  exchange package and the client, and nothing else of the core. A Go test walks the
  import graph and fails if that is ever untrue — `make verify` already runs
  `go test ./...`, so the rule is checked on every build and in CI.
- **Presentation leaves the API.** Display words, glyphs, help text, colour choice
  and drawing hints move to the shared UI-neutral module `internal/ui/theme`, which
  both front-ends already import — so `ui/theme` stops importing `internal/hub/chat`
  and the dependency points the right way.
- **The tree and section models live with the types they operate on.** Task
  arrangement and the resolved section list belong to `internal/api`, used by the hub
  and every front-end alike, rather than being the hub's to compute for its clients.
- **The front-ends stop reaching past the API.** The openspec-availability notice
  comes from the hub, which is the process that needs the tool; hub liveness and
  uptime come from the board; `agent.SyncPodBin()` moves into the hub binary's own
  startup, where hub-side setup belongs.

## Capabilities

### Modified Capabilities

- `01-architecture`: the hub hosts the logic and front-ends are clients — the
  exchange format is a dependency-free package both sides import, no front-end
  package imports the service, and the rule is enforced by a test rather than
  convention. The UI-neutral rendering module owns all presentation, including what
  the hub currently exports.
- `hub`: the hub runs as its own binary that interactive entry points exec; it serves
  resolved sections rather than a section model whose counts are functions; and task
  arrangement is a function of the exchange package rather than the hub.
- `view-tui`: the TUI reaches the hub only through the client and the exchange
  package, and computes its own presentation from data rather than receiving display
  strings.

## Impact

- **New:** `internal/api` (types + pure functions, no imports), `cmd/sindri-hub`
  (thin entrypoint), `internal/client` (moved from `internal/hub/client`, which
  `cmd/sindri-worker` also uses), an import-guard test under `internal/ui`.
- **Moved:** the 8 wire-bearing types out of `internal/hub/store`; `Config`/`GitHub`/
  `Lint` out of `internal/config`; `PriorityLabel`/`StateLabel`/`FormatClients`/the
  chat glyphs and help text into `internal/ui/theme`; `ArrangeTasks`/`Descendants`
  and the resolved section list into `internal/api`.
- **Deleted:** the re-export block in `internal/hub/wiring.go` (the seam adapters
  stay), `hub.New()` from `internal/ui/cli/hub.go`.
- **Packaging:** `Makefile`, `scripts/install.sh`, the release tarball and the README
  install section each enumerate the shipped binaries and gain the fourth.
- No protocol change: the same JSON crosses the socket, and the types that describe
  it simply stop belonging to the server.
