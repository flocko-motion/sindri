# Tasks

## 1. Make both front ends remove an orphan

- [x] 1.1 CLI: `sindri agent delete <name>` removes a reported orphan. An orphan has no
      roster entry, so the agent lookup cannot resolve it — the board's orphan list is what
      decides, and the message says an orphan was removed rather than an agent deleted.
- [x] 1.2 CLI: `agent list` stops printing `podman rm -f <name>` and names the sindri
      command instead, so the interface no longer depends on the engine's syntax.
- [x] 1.3 TUI: the orphan detail line stops suggesting `podman rm -f`; removal was already
      bound to the delete key, behind a confirmation.

## 2. Confirm the specs match

- [x] 2.1 `openspec validate --all` passes.
- [x] 2.2 The hub still never removes an orphan unprompted — this change adds no sweep. Audited
      every caller of `container.Rm`, which is the only thing that actually removes a container:
      `DeleteAgent`, `StopAgent` and `Launch` (clearing a stale container of the same name) all
      act on a DECLARED agent, so by the requirement's own definition none of them touches an
      orphan; `RemoveOrphan` is reached only from `POST /orphan/remove`, which only a front end
      calls, and both front ends confirm first. No timer, watchdog or sweep calls any of them.
