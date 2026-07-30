## ADDED Requirements

### Requirement: Container runtime is a pluggable backend behind one port

The hub SHALL reach the container runtime through a single port (interface), with interchangeable backends, so that no hub, CLI, or TUI code depends on a specific runtime CLI. At least two backends SHALL satisfy the port: podman (a shared Linux VM) and Apple `container` (a per-container micro-VM, macOS). The backend SHALL be selectable by configuration. On macOS the default SHALL be Apple `container` — its per-agent micro-VM is what satisfies the isolation requirement below — with podman available as an opt-out (`SINDRI_RUNTIME=podman`); on Linux the backend SHALL always be podman, since Apple `container` requires macOS.

#### Scenario: A caller drives the runtime through the port

- **WHEN** the hub launches, execs into, attaches to, or tears down an agent's pod
- **THEN** it calls the runtime port, and the concrete backend (podman or Apple `container`) is chosen once from configuration — no caller references a specific runtime CLI

#### Scenario: Backend is swapped without touching callers

- **WHEN** the configured runtime backend changes
- **THEN** only the backend selection changes; the hub, CLI, and TUI code is unaffected because they use the port

#### Scenario: Default and platform constraint

- **WHEN** no runtime is configured on macOS
- **THEN** the Apple `container` backend is used (opt out with `SINDRI_RUNTIME=podman`)
- **WHEN** the host is Linux, or podman is explicitly selected
- **THEN** the podman backend is used

### Requirement: One agent's runtime failure is isolated from other agents

An agent SHALL run in its own runtime instance such that the crash, out-of-memory, or wedge of one agent's runtime does not take down other agents. A backend that shares a single VM across all agents does not satisfy this (one VM failure ends the whole fleet); a backend that gives each container its own micro-VM does.

#### Scenario: One agent's runtime dies

- **WHEN** a single agent's runtime instance crashes or is OOM-killed
- **THEN** the other agents keep running, and only the failed agent is reported down

#### Scenario: Shared-VM backend is a known limitation

- **WHEN** the shared-VM backend (podman) is in use and its VM fails
- **THEN** the fleet-wide failure is understood as that backend's limitation — the per-container-VM backend exists to avoid it
