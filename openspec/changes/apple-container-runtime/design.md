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

- **The port lives in the core (hexagonal).** `internal/container` IS the abstraction:
  it declares the `Runtime` interface — `Run`, `Exec`, `ExecContext`,
  `ExecInteractive`, `Running`, `RunningContext`, `Logs`, `Info`, `Rm`, `ListByLabel`,
  `Check`, `Healthy`, `EnsureImage`, `ImageName` — plus the shared value types
  (`RunOpts`, `Mount`) and the shared, backend-agnostic build recipe (the embedded
  buildctx, materialize, build-key, `ImageName`). It imports **no** adapter.
- **Adapters implement the port.** `internal/adapter/pod` (podman) and
  `internal/adapter/applecontainer` become `Engine` types implementing
  `container.Runtime`; each imports `internal/container` (for the types + shared build
  helpers) and owns its CLI quirks (argv, output parsing, label filtering, the actual
  `build` invocation). The adapters do **not** depend on each other.
- **Core depends only on the abstraction.** `internal/hub`, `cmd/sindri`, and
  `internal/tui` call `internal/container` (the port), never a specific adapter.
- **Wiring at the composition root.** `cmd/sindri` (main) is the only place that
  imports the adapters; it picks one from config and injects it via
  `container.Use(engine)`. Platform default: podman everywhere, `container` opt-in on
  macOS; Linux forces podman. Resolved once at startup.
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

## Spike findings (Apple `container` 1.0.0, macOS 26) — 2026-07-06

Verified against `container` CLI 1.0.0 with a throwaway alpine container:

- **Flag surface maps to podman**: `-d`, `-v`/`--mount`, `-l`/`--label`, `--name`,
  `-e`, `-i`/`-t`, `-w`, `--entrypoint`, `--rm`. `RunArgs` translation is mechanical.
- **Bind mounts work**: `-v /tmp:/host` + `container exec … cat /host/<file>` read a
  host-written file. The overlay-image-root + bind-mounted-worktree model holds.
- **Per-container isolation confirmed** (the point): each container gets its own IP
  (`192.168.64.x`, gateway `.1`) and its own memory (default 1024 MB). One
  container's OOM/crash can't touch another.
- **Running state**: `container inspect <name>` → `.status.state == "running"` (also
  in `container ls --format json`). Maps `Running()`.
- **`logs`, `rm -f`, `exec` work.** Service lifecycle: `container system start`
  (installs a kata Linux kernel once) — so this backend's `Healthy` probes
  `container system status`.
- **DIFFERENCE — no `container ls --filter label=`.** Orphan detection (`ListByLabel`)
  can't use a native filter; the backend must `container ls --all --format json` and
  match labels in Go. (Labels live under each entry's `configuration`.)
- **OPEN — micro-VM → hub networking (task 1.4).** The agent gets its own IP and the
  host is the gateway (`192.168.64.1`), so the hub's TCP agent channel must listen on
  an address reachable from the container network (e.g. `0.0.0.0` / the gateway), not
  only `127.0.0.1`. To confirm with a real hub + agent during task 3.
- **OPEN — image build (`container build`) and interactive `exec -it` attach**
  (TTY/resize/signals) not yet exercised end to end; low risk (buildkit + podman-like
  flags), confirm during task 3.
