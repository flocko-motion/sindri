## Context

The agent domain model currently lives in `internal/hub`:

- `internal/hub/state.go` defines `AgentView` and `ClientView`, and the pure helpers
  `parseClients` and `FormatClients`, alongside the orchestration-heavy `State`
  board read.
- The container-name identity `hub.Container(root, name)` also lives in `hub`.
- `internal/client.Clients` returns `[]hub.ClientView`; `cmd/sindri` (backend
  interface, `agent.go`, `attach.go`) and `internal/tui` (`tab_agents.go`,
  `messages.go`, `refresh.go`, `tui.go`) all import `hub` solely to name these
  types or call `Container`/`FormatClients`.

The `01-architecture` spec already mandates headless logic, a thin UI layer, and a
UI-neutral rendering module. This change applies the same reasoning one level down:
the domain *model* should not be embedded in the orchestrating service package.

## Goals / Non-Goals

**Goals:**
- A pure `internal/agent` package owning the agent model and pure logic.
- `hub`, `client`, `cmd/sindri`, and `internal/tui` all depend on `internal/agent`
  for the model; UIs stop importing `hub` just for a data type.
- No behavior change and no wire-format change: `BoardState` still serializes the
  same fields; a mismatched hub/CLI keeps interoperating.

**Non-Goals:**
- Moving orchestration (launch, serve, liveness probes, routing, workflows, board
  assembly) out of `hub`. Those act on agents via `store`/`pod` and stay put.
- Adding agent *lifecycle* methods to `internal/agent`. It must not grow
  dependencies on `store` or `pod`.
- Renaming the JSON fields or changing any HTTP endpoint.

## Decisions

- **What moves to `internal/agent`** (the "what an agent *is*" logic):
  - `AgentView`, `ClientView` types.
  - `Container(root, name string) string` — the deterministic pod name.
  - Status collapsing (the `down|idle|working|collab|launching|stopping` derivation
    that turns runtime + workflow phase into one word), as a pure function taking
    the inputs it needs (running bool, phase, etc.) rather than a `*Hub` method.
  - `parseClients(string) []ClientView` and `FormatClients([]ClientView) string`.
- **What stays in `hub`**: `State`/board assembly, `clientsCtx`/`Clients` (they call
  `pod.ExecContext`), liveness probes, launch/serve, routing, workflows. `hub`
  constructs `agent.AgentView` values and calls `agent`'s pure helpers.
- **Type aliases vs. move**: do a hard move, then update imports. Optionally keep a
  short-lived `type AgentView = agent.AgentView` alias in `hub` only if it
  meaningfully shrinks the diff; prefer updating references directly so there's one
  name for each type.
- **`BoardState` location**: `BoardState` stays in `hub` (it aggregates tasks, PRs,
  projects, orphans — hub concerns) but its `Agents` field becomes
  `[]agent.AgentView`.
- **Package name**: `agent`, imported as `agent`; avoids collision with the CLI's
  `newAgentCmd` wiring (different package, `main`).

## Risks / Trade-offs

- **Wide, shallow diff.** Many files change imports/type qualifiers. Mitigate by
  moving types first, then letting the compiler list every reference to update; the
  `make verify` gate (build + test + lint) confirms completeness.
- **Merge overlap with the multi-agent worktree work**, which touches the same
  `hub`/`tui` files. Trade-off: sequence this change deliberately (land it on its
  own, rebase the other work) rather than interleaving.
- **Import cycles.** `agent` must import neither `hub` nor `store`/`pod`. The status
  helper therefore takes primitives as parameters, not a `*Hub`. If a helper seems
  to need the store, that's the signal it's orchestration and belongs in `hub`.
- **Over-extraction.** Keep the boundary at "pure model"; resist moving anything
  that touches adapters, or the dependency-light property is lost.
