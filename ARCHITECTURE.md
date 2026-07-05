# Architecture

Sindri follows a **strict hexagonal architecture** (ports & adapters). These rules
are not aspirational — they are enforced in review.

## The core is headless

All domain logic lives in the core (`internal/hub` and the packages it uses) and
is **headless and interface-agnostic**. It knows nothing about how it is invoked:
no terminal, no key handling, no rendering, no flags. It exposes operations; it
does not care who calls them.

## The outside world is reached only through adapters

Every interaction with something outside the process — git, podman, tmux, the
task tracker (td), the spec tool — goes through an adapter in `internal/adapter/`.
The core calls adapters; it never shells out, dials a socket, or touches an
external tool directly. Swapping or mocking an external tool is a change to one
adapter and nothing else.

## CLI and TUI are interchangeable front-ends

The CLI (`cmd/sindri`) and the TUI (`internal/tui`) are **thin interface layers**.
They must always execute the **same** core logic — they translate user intent into
core operations and render the result. Nothing more.

- **No business logic in the CLI or TUI.** Only interface logic belongs there:
  argument parsing, key handling, layout, rendering, formatting.
- Any behaviour offered by one front-end must be reachable from the other, because
  both drive the same operations.
- A front-end reaches the core through the client (`internal/client`), which talks
  to the single hub — so the CLI and TUI are literally running the same code.

## Topology

There is **one hub per machine** — the single global coordinator that owns all
state and drives every agent across every repo. Agents run in **podman pods**, one
per agent, isolated in their own git worktrees. Front-ends and agents reach the hub
over its unix socket (over TCP on macOS, where the podman VM can't cross a
bind-mounted socket). The hub is the only process with domain logic; everything
else is a front-end or a pod.

## Tooling

- `cmd/sindri` — the product: the host CLI and the TUI launcher.
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
