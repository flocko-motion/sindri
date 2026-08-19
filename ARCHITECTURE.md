# Architecture

Sindri follows a **strict hexagonal architecture** (ports & adapters). These rules
are not aspirational. Several are enforced by tests that fail the build (each named
below where it applies); the rest are enforced in review. A rule you can break
without a test failing is still a rule.

## The core is headless

All domain logic lives in the core (`internal/hub` and the packages it uses) and
is **headless and interface-agnostic**. It knows nothing about how it is invoked:
no terminal, no key handling, no rendering, no flags. It exposes operations; it
does not care who calls them.

## The outside world is reached only through adapters

Every interaction with something outside the process — git, podman, tmux, GitHub,
the spec tool — goes through an adapter in `internal/adapter/`. The core calls
adapters; it never shells out, dials a socket, or touches an external tool directly.

**Name the port, not the tool.** Where a family of implementations exists for one
job — the container runtime, the coding agent, the task source — the core depends on
the **port**, the interface naming that job, and never names a concrete adapter.
Choosing the implementation is the composition root's work: the core is handed one
and never constructs it.

Where a tool has exactly one implementation and no plausible second — git, tmux —
the core imports its adapter directly. The abstraction earns its place by making an
implementation swappable or fakeable; requiring one where nothing can vary buys
nothing and hides which tool is in use.

**A port hides its differences, or it is not a port.** Sindri's own tasks, openspec
changes and GitHub issues are all tasks: each is worked, closed, submitted and
reviewed identically, and no caller may branch on which kind it holds. Where the
kinds genuinely differ — a task sindri owns keeps its status in the hub's store, a
mirrored one keeps it at its source — that difference lives behind one operation
(`workflow.Engine.SetStatus`) and stops there. A caller that has to ask what kind of
task it has is a caller that will one day forget to; four of them did, each leaving a
finished openspec change reading open. `internal/hub/workflow/onehome_test.go` holds
the line for status specifically: only the owned source may write `owned_tasks`.

## CLI and TUI are interchangeable front-ends

The CLI and the TUI (both under `internal/ui`, launched by `cmd/sindri`) are **thin
interface layers**.
They must always execute the **same** core logic — they translate user intent into
core operations and render the result. Nothing more.

- **No business logic in the CLI or TUI.** Only interface logic belongs there:
  argument parsing, key handling, layout, rendering, formatting.
- Any behaviour offered by one front-end must be reachable from the other, because
  both drive the same operations. `internal/ui/parity_test.go` enforces it over the
  client's method set; a deliberate exception goes in that test's allowlist with the
  reason it is not a gap, so the argument is on record rather than assumed.
- A front-end reaches the core through the client (`internal/client`), which talks
  to the single hub — so the CLI and TUI are literally running the same code.
- **A front-end links no hub code.** It carries the exchange format (`internal/api`)
  and the client, and no hub package, persistence driver, or adapter that only the hub
  needs. An adapter the front-end itself needs is fine — it attaches to tmux and runs
  git locally. A front-end that needs a hub running starts it by executing that
  binary, rather than constructing one in its own process.
  `internal/ui/importguard_test.go` enforces the `internal/hub` half of this over the
  real import graph, so an indirect import fails too.

## Every file declares its layer

Each non-test `.go` file opens with the four-field header `brokkr map` reads, and its
`type:` names the layer from a **closed set**: `logic`, `adapter`, `assembly`,
`rendering`, `ui`, `command`, `entrypoint`. Closed on purpose — a vocabulary anyone
may extend describes nothing, and a file that fits none of the seven is usually a
file doing two jobs. `internal/arch/vocab_test.go` fails the build on an eighth.

## Context is handed through, never invented

`context.Background()` belongs at an **entrypoint** — a `main`, a server's request
root, a background loop where it starts. Everywhere below that, a function takes the
caller's context and passes it on, narrowing it where the work needs its own bound:
`context.WithTimeout(ctx, probeTimeout)` around a probe, never a fresh root.

Inventing one at depth cuts two wires at once. Cancellation stops reaching the work,
so an operation the caller has abandoned runs on; and a root carries no deadline to
inherit, so a wedged dependency blocks its caller for ever. Both have happened here —
a liveness probe on `context.Background()` held every board read open until podman
answered, which under load it did not.

In the hub this resolves into three lineages, and every call belongs to one of them.
A **read** takes the request's context, so abandoning the request abandons the work.
Work the hub must **finish** whatever the client then does — a pod coming up, a pod
going away, a message landing — takes that context with its cancellation dropped
(`hub.detached`), which keeps the lineage visible at the handler where the decision
belongs. **Fleet-side** work the hub does on its own account runs under the hub's
**lifetime** (`Hub.lifetime`, cut from the root `main` hands to `hub.New`): its loops,
and the pushes it makes through a port whose signature carries no context of its own.

`internal/arch/context_test.go` holds the line: a file that starts a context must be on
its list with the reason it is an entrypoint, and an entry that stops rooting anything
has to go — so the argument stays on record rather than being remembered.

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
