# Show the fleet's memory headroom

## Why

Running many agents is a memory question, and nothing on screen answers it. Per-agent usage is
measured — `Runtime.Stats` reports what a pod uses against its ceiling, `agent stats` prints it, and
the board already carries `DefaultMemory` — but the fleet figure is missing: how much the machine
has left, and therefore whether another agent will start.

A percentage would leave the reader dividing free memory by a default they would have to look up.
The number worth showing is the one the question is about: what is free, and how many more agents of
the default size fit in it.

The host cannot be read for that figure, because "left" means a different thing per backend.
Containers sharing the host kernel take memory as they use it, so the machine can be overcommitted
and a limit costs nothing until it is spent. A micro-VM reserves its whole limit when it starts, so
an agent whose reservation does not fit is refused however quiet the fleet is. And podman on macOS
runs its containers inside a VM, so the ceiling is that VM's allocation — the Mac's own RAM would
name a ceiling no container can reach. The runtime port already answers `DefaultMemory` for this
reason; capacity is the same question one step out.

## What changes

- `container.Runtime` gains `MemoryCapacity`, beside `DefaultMemory` and `Stats`: what the fleet
  costs the machine against the ceiling it draws from, with a word for what the used figure counts.
  Podman answers from `podman info`, which describes the host it runs containers on. Apple
  `container` answers with the sum of the running micro-VMs' reservations against the Mac's memory.
- The hub folds one reading into `BoardState.Memory`: free memory, and how many more default-size
  agents fit there. It sits beside `DefaultMemory` and `StartedAt`, being a property of the machine
  and its runtime rather than of a project.
- The reading is taken by the watchdog on a slower cadence of its own, so a board read reports it
  rather than paying a process spawn for it — the rule liveness already follows.
- The TUI draws it top-right in the header, reusing the existing memory meter. It degrades as the
  terminal narrows — meter first, then the free figure — and disappears rather than push a tab off
  the edge.
- `sindri agent stats` opens with the same figure, above the per-agent rows it is usually read
  beside.

## Impact

- Specs: `agent-runtime` (the capacity question and where it is answered), `view-tui` (the header).
- Code: `internal/container`, both container adapters, `internal/hub` (`watchdog.go`, `state.go`,
  `agent/memory.go`), `internal/api/board.go`, `internal/ui/theme/mem.go`, `internal/ui/tui`,
  `internal/ui/cli/agent.go`.
- A machine nobody has measured yet — the first seconds of a hub, a runtime that cannot answer —
  carries no figure, and both front-ends draw nothing rather than a full machine.
- No history, no graph, and no per-agent breakdown in the header: `agent stats` already covers
  per-agent detail.
