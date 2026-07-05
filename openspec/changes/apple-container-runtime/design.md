## Context

`internal/adapter/pod` is the podman adapter, and its exported surface is what the
rest of sindri needs from a container runtime:

- `Run(RunOpts)` / `RunArgs` (detached pod: image, mounts, labels, env, entrypoint)
- `Exec` / `ExecContext` (non-interactive), `ExecInteractive` (attach, TTY)
- `Running` / `RunningContext`, `Logs`, `Info`, `Rm`, `ListByLabelContext`
- `Check` / `Healthy` (VM/runtime reachability pre-flight)
- plus `internal/container.Ensure` (build the agent image) and `ImageName`

The hub, CLI, and TUI call `pod.*` directly (main users: `pod.Running`, `pod.Mount`,
`pod.Rm`, `pod.Exec`, `pod.ExecInteractive`, `container.Ensure`). Adding Apple
`container` means these callers must go through a port, not a CLI-specific package.

Apple `container` (macOS 26, github.com/apple/container) runs Linux OCI containers,
one **micro-VM per container**, with its own CLI (`container run/exec/ls/build/logs/
rm/…`). The overlay image root + bind-mounted worktree model is the same as podman;
the CLI verbs, flags, `inspect`/`ls` output formats, label filtering, and exec/TTY
semantics differ.

## Goals / Non-Goals

**Goals:**
- One `Runtime` port covering the surface above; the hub/CLI/TUI depend only on it.
- podman refactored into a backend implementing the port (default; Linux-only path).
- An Apple `container` backend implementing the same port on macOS.
- Runtime chosen once from config; per-agent failure isolation with the micro-VM
  backend.

**Non-Goals:**
- Changing the agent image, the overlay architecture, or the agent↔hub protocol.
- Dropping podman — it stays the default and the Linux runtime.
- Native-macOS / non-Linux execution (settled: sindri needs overlayfs → Linux).

## Decisions

- **The port.** A `Runtime` interface (likely in a new `internal/adapter/runtime`,
  or promoted within `internal/adapter/pod`) with methods mirroring today's surface:
  `Run`, `Exec`, `ExecContext`, `ExecInteractive`, `Running`, `RunningContext`,
  `Logs`, `Info`, `Rm`, `ListByLabel`, `Healthy`/`Check`, and image `Ensure`. Shared
  value types (`RunOpts`, `Mount`) move to the port package. Image building is part
  of the port (podman `build` vs `container build`).
- **Backends.** `podman` backend = today's code behind the interface. `applecontainer`
  backend = the same methods mapped to the `container` CLI. Each backend owns its CLI
  quirks (arg construction, output parsing, label filtering).
- **Selection.** A single startup choice: config key (e.g. `runtime: podman|container`)
  with a platform default (podman everywhere; `container` opt-in on macOS). Resolved
  once into the concrete `Runtime`, injected into the hub. Linux forces podman.
- **Identity/naming.** Container naming (`hub.Container`) and label scheme stay; each
  backend maps them to its `ls --filter`/inspect. Orphan detection (`ListByLabel`)
  must work on both.
- **Health pre-flight.** `Healthy` per backend: podman probes the VM (existing);
  `container` probes the `container system` service. The `agent` subcommand warning
  already added calls the port's `Healthy`.
- **Attach / TTY.** `ExecInteractive` maps to `container exec -it` (verify TTY +
  signal handling parity with `podman exec -it`).

## Risks / Trade-offs

- **Apple `container` maturity/parity.** It's new (2025). Before committing, a spike
  must verify the port maps: detached run with host bind mounts, `exec`, interactive
  `exec -it` (attach), running-state inspect, logs, `rm`, **label-based listing** for
  orphan detection, and `build`. Any gap (esp. label filtering / inspect format /
  TTY) shapes the backend or forces a shim.
- **Networking.** Per-micro-VM means no shared unix socket — sindri already uses the
  TCP agent channel on macOS, which covers this; verify the hub is reachable from a
  `container` micro-VM (its own IP) and the token auth path holds.
- **Two code paths to maintain.** Mitigate by keeping the port minimal and pushing
  all CLI specifics into the backends; a shared conformance test suite runs the same
  scenarios against whichever backend is present.
- **macOS-26-only.** The `container` backend needs macOS 26 + the tool installed;
  `Healthy` must give an actionable message when selected but unavailable, and
  selection must refuse `container` on Linux.
- **Scope creep.** The abstraction (port + podman backend) is worthwhile on its own;
  land it first, then add the `container` backend behind it — so the refactor is not
  gated on Apple-`container` verification.
