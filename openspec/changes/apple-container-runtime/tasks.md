## 1. Spike: verify Apple `container` maps to sindri's needs

- [x] 1.1 On macOS 26, install Apple `container`; confirm the agent OCI image builds (`container build`) or loads, and runs detached with a **host bind mount** (worktree + `~/.claude`).
- [x] 1.2 Verify `exec` and interactive `exec -it` (attach): TTY, resize, and signal behaviour match `podman exec -it` closely enough for the tmux-attach flow.
- [x] 1.3 Verify running-state inspect, `logs`, `rm`, and **label-based listing** (needed for orphan detection). Note any format/flag differences.
- [ ] 1.4 Verify the hub reaches an agent in a `container` micro-VM over the existing TCP agent channel (token auth), since a shared unix socket won't cross the micro-VM boundary.
- [x] 1.5 Write up gaps; if any primitive is missing, decide shim vs. defer.

## 2. Define the runtime port (podman-only first)

- [ ] 2.1 Introduce a `Runtime` interface covering: Run, Exec, ExecContext, ExecInteractive, Running, RunningContext, Logs, Info, Rm, ListByLabel, Healthy/Check, and image Ensure. Move `RunOpts`/`Mount` to the port package.
- [ ] 2.2 Refactor the current podman code in `internal/adapter/pod` into a `podman` backend implementing the port (behaviour unchanged).
- [ ] 2.3 Route `internal/container.Ensure`/`ImageName` through the port (build is backend-specific).
- [ ] 2.4 Update callers in `internal/hub`, `cmd/sindri`, `internal/tui` to use the port (injected `Runtime`), not `pod.*` directly.
- [ ] 2.5 Runtime selection: a config key (default podman; Linux forced to podman), resolved once at startup into the concrete `Runtime`. `make verify` green; behaviour identical to today.

## 3. Apple `container` backend

- [ ] 3.1 Implement the `container` backend against the port (run/exec/exec-it/running/logs/info/rm/list-by-label/healthy/build), per the spike findings.
- [ ] 3.2 `Healthy` for this backend probes the `container` service and gives an actionable hint when selected-but-unavailable; selection refuses it on Linux and on pre-26 macOS.
- [ ] 3.3 A conformance test suite runs the same scenarios (run→exec→attach→logs→rm→orphan-list) against whichever backend is present, so both stay in parity.

## 4. Verify

- [ ] 4.1 `make verify` green with podman backend (default) — no regression.
- [ ] 4.2 On macOS 26 with the `container` backend selected: launch two agents, kill/OOM one, and confirm the **other keeps running** (the isolation the change exists for) and the failed one shows `down`.
- [ ] 4.3 Confirm switching the runtime config flips backends with no hub/CLI/TUI code change.
