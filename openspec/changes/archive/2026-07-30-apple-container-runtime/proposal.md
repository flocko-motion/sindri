# Pluggable container runtime: podman + Apple `container` behind one port

## Why

All agents share **one** podman Linux VM. When it runs out of memory or wedges,
**every agent goes down at once** — the failure domain is the whole fleet, which is
exactly the pain in practice (a 2 GiB VM dies under a few Claude agents, taking all
of them with it). Apple's `container` (macOS 26) runs **one micro-VM per
container**, so an agent's crash/OOM is contained to that agent — the isolation
sindri wants, keeping the overlay-based Linux-container architecture intact (each
micro-VM is a real Linux kernel with overlayfs).

But sindri talks to **podman directly** (`internal/adapter/pod` hard-codes the
podman CLI; the hub/CLI/TUI call it). Adding a second runtime means first
abstracting the runtime into a **port with interchangeable backends** — podman and
Apple `container` — selectable without touching the hub or UIs.

## What Changes

- **A single runtime port.** Define the container-runtime contract sindri needs
  (run, exec, exec-interactive, running, logs, info, remove, list-by-label,
  reachability pre-flight, image build) as one interface. The hub, CLI, and TUI
  reach the runtime only through it — no caller names a specific runtime CLI.
- **podman becomes one backend** behind the port (the current adapter, refactored
  to implement it) — the default, and the only option on Linux.
- **Apple `container` is a second backend** implementing the same port on macOS via
  the `container` CLI (per-container micro-VMs). The overlay image + bind-mounted
  worktree model is preserved.
- **Runtime is selectable** (config / platform default): on macOS the default is
  Apple `container` (per-agent micro-VM isolation — the point of this change); opt
  out to podman with `SINDRI_RUNTIME=podman`. Linux always uses podman.
- **Failure isolation becomes a stated property**: an agent's runtime failure must
  not take down other agents — met by per-container-VM backends, and the reason the
  shared-VM default is a liability at scale.

## Capabilities

### New Capabilities
<!-- none — this restructures how the existing agent runtime is reached -->

### Modified Capabilities
- `agent-runtime`: the container runtime becomes a pluggable backend reached through
  one port (podman and Apple `container` interchangeable), and gains the property
  that one agent's runtime failure is isolated from the others.

## Impact

- **`internal/adapter/pod`**: extract the runtime contract into a port; the current
  podman code becomes the `podman` backend implementing it.
- **New `internal/adapter/*` backend**: an `apple-container` (or `container`) backend
  implementing the same port via the `container` CLI.
- **`internal/container` image build** (`Ensure`) routed through the port (podman
  build vs `container build`).
- **Call sites** in `internal/hub`, `cmd/sindri`, `internal/tui` reference the port,
  not `pod.*` directly; runtime is chosen once at startup.
- **Config**: a runtime-selection setting (macOS default: Apple `container`; podman
  elsewhere and via opt-out). The Apple backend requires macOS 26 + `container` installed.
- No change to the agent image, the overlay model, or the agent↔hub protocol (the
  macOS TCP channel already crosses the VM boundary and covers per-micro-VM too).
