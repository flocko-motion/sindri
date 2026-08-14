# Tasks

## 1. Ask the backend, not the host

- [x] 1.1 `container.Runtime` gains `MemoryCapacity`, returning what the fleet costs the machine
      against its ceiling, with a word for what the used figure counts.
- [x] 1.2 Podman answers from `podman info`, which describes the host it runs containers on — the
      VM's memory on macOS, and the machine's on Linux.
- [x] 1.3 Apple `container` answers with the running micro-VMs' reservations against the Mac's
      memory, since a reservation is held whether it is used or not.
- [x] 1.4 The unwired backend answers with an error, so a process with no runtime reports no figure
      rather than an empty machine.

## 2. One reading, folded once

- [x] 2.1 `agent.Headroom` turns a reading into the board's figure: free memory, and how many more
      agents of the default size fit there.
- [x] 2.2 The default's size is parsed from the limit the runtime states, so the count is in the
      unit an agent is actually launched with.
- [x] 2.3 The watchdog takes the reading on its own slower cadence, off the liveness loop, and the
      board reports the last one taken.
- [x] 2.4 `BoardState.Memory` carries it, beside `DefaultMemory` and `StartedAt`.

## 3. Both front-ends render the same figure

- [x] 3.1 `theme.FleetLine` and `theme.FleetBadge` render it, so the CLI and the TUI cannot drift.
- [x] 3.2 The TUI header draws the badge top-right, reusing the existing meter, degrading as the
      terminal narrows and disappearing before a tab does.
- [x] 3.3 `sindri agent stats` opens with the fleet line, above the per-agent rows.

## 4. Pin it

- [x] 4.1 The fold: agents counted rather than a percentage, an unmeasured machine left unknown, an
      overcommitted one fitting nothing.
- [x] 4.2 The parse, over every form a limit is written in — the fit count divides by it.
- [x] 4.3 Each backend's reading, against the shape its tool really prints.
- [x] 4.4 The board carries the watchdog's last reading, and carries none before the first.
- [x] 4.5 The header at every width: the figure shown where there is room, shortened where there is
      little, and gone before a tab is pushed off the edge.
