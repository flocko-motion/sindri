# Architecture

Sindri follows a **strict hexagonal architecture** (ports & adapters). These rules
are not aspirational — they are enforced in review.

## The core is headless

All domain logic lives in the core (`internal/hub` and the packages it uses) and
is **headless and interface-agnostic**. It knows nothing about how it is invoked:
no terminal, no key handling, no rendering, no flags. It exposes operations; it
does not care who calls them.

## The outside world is reached only through adapters

Every interaction with something outside the process — git, podman, tmux, GitHub,
the spec tool — goes through an adapter in `internal/adapter/`. Tasks are the
exception by ownership rather than by layering: sindri holds its own in the hub's
store, and adapters cover only the trackers it mirrors.
The core calls adapters; it never shells out, dials a socket, or touches an
external tool directly.

**Name the port, not the tool.** Where a family of implementations exists for one
job — the container runtime, the coding agent, the task source — the core depends on
the **port**, the interface naming that job, and never names a concrete adapter.
Choosing the implementation is the composition root's work: the core is handed one
and never constructs it.

Where a tool has exactly one implementation and no plausible second — git, tmux —
the core imports its adapter directly. The abstraction earns its place by making an
implementation swappable or fakeable; requiring one where nothing can vary buys
nothing and hides which tool is in use.

## CLI and TUI are interchangeable front-ends

The CLI and the TUI (both under `internal/ui`, launched by `cmd/sindri`) are **thin
interface layers**.
They must always execute the **same** core logic — they translate user intent into
core operations and render the result. Nothing more.

- **No business logic in the CLI or TUI.** Only interface logic belongs there:
  argument parsing, key handling, layout, rendering, formatting.
- Any behaviour offered by one front-end must be reachable from the other, because
  both drive the same operations.
- A front-end reaches the core through the client (`internal/client`), which talks
  to the single hub — so the CLI and TUI are literally running the same code.
- **A front-end links no hub code.** It carries the exchange format (`internal/api`)
  and the client, and no hub package, persistence driver, or adapter that only the hub
  needs. An adapter the front-end itself needs is fine — it attaches to tmux and runs
  git locally. A front-end that needs a hub running starts it by executing that
  binary, rather than constructing one in its own process.
  `internal/ui/importguard_test.go` enforces the `internal/hub` half of this over the
  real import graph, so an indirect import fails too; the rest is review's to hold.

## Topology

There is **one hub per machine** — the single global coordinator that owns all
state and drives every agent across every repo. Agents run in **podman pods**, one
per agent, isolated in their own git worktrees. Front-ends and agents reach the hub
over its unix socket (over TCP on macOS, where the podman VM can't cross a
bind-mounted socket). The hub is the only process with domain logic; everything
else is a front-end or a pod.

## Tooling

- `cmd/sindri` — the product: the host CLI and the TUI launcher.
- `cmd/sindri-hub` — the hub itself: the single writer, run in the foreground or
  spawned detached by `sindri hub start`.
- `cmd/sindri-worker` — the in-pod worker that runs the agent inside its pod.
- `cmd/brokkr` — a **separate** binary for generic dev tooling (code map, linters),
  kept out of the product.
- `openspec/` — specs drive behaviour changes and are validated as part of the
  quality gate.

## Conformance

All code must conform to the built-in `brokkr` linters (`brokkr lint`, run by
`make verify`). That baseline is enforced automatically — a change that fails it
does not merge.

## Why

Behaviour is defined once, testable without a terminal, and identical no matter how
it is driven. A change to what sindri *does* lands in the core; a change to how it
*looks* or is *typed* lands in a front-end.
