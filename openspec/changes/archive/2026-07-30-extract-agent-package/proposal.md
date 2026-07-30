# Extract a pure `internal/agent` domain package

## Why

An agent is the *subject* the hub manages, not a member of the hub — yet the agent
domain model lives inside `internal/hub`. So `cmd/sindri` and `internal/tui` import
the whole orchestrator package just to name an `AgentView`, a `ClientView`, or
compute a container name. UI adapters reaching into the service package for plain
data types is backwards layering, and it keeps the already-large `hub` package
carrying concerns that aren't orchestration. As we stand up the multi-agent
worktree workflow, the agent model wants a clear, dependency-light home.

## What Changes

- **New `internal/agent` package** holding the agent *domain model and pure logic*:
  `AgentView`, `ClientView`, the `Container(root, name)` identity/naming, status
  collapsing (`down|idle|working|collab`), and `parseClients`/`FormatClients`. It
  depends on nothing from `hub`, `store`, or `pod`.
- **`hub` imports `agent`.** All orchestration that *acts on* agents via adapters —
  launch, serve, liveness probes, routing, workflows, board assembly — stays in
  `hub`, now expressed in terms of the `agent` model.
- **UIs depend on the model, not the service.** `cmd/sindri` and `internal/tui`
  reference `agent.AgentView`/`agent.ClientView`/`agent.Container` from the small
  domain package instead of importing `hub` for those types.
- **No behavior change.** This is a structural move: the same values flow to the
  same UIs. `BoardState` still carries `[]agent.AgentView`; the JSON on the wire is
  unchanged, so a mismatched hub/CLI keeps working across the boundary.

## Capabilities

### New Capabilities
<!-- none — this introduces no user-facing capability; it re-homes existing domain types -->

### Modified Capabilities
- `01-architecture`: add a requirement that the shared domain model live in its own
  package that both the service (hub) and every UI depend on — a UI SHALL NOT
  import the orchestrating service package merely to reference a domain type — and
  that this domain package stay free of adapter/service dependencies.

## Impact

- **New package**: `internal/agent` (types + pure functions moved out of
  `internal/hub/state.go` and the container-name helper).
- **Touched for imports/type references**: `internal/hub` (state, server, workflows),
  `internal/client` (`Clients` return type), `cmd/sindri` (`agent.go`, `attach.go`,
  `hub.go` backend interface), `internal/tui` (`tab_agents.go`, `messages.go`,
  `refresh.go`, `tui.go`).
- **No API/behavior/wire-format change**; no data migration. Overlaps the files the
  multi-agent worktree work touches, so best sequenced deliberately.
