# Hot-swappable agent tooling and instructions

## Why

Evolving an agent is expensive, in two distinct ways:

- **Instructions are baked into the image.** Skills (`container/skills/*`) and
  `CLAUDE.md` are `COPY`-d in at build time, so changing a single line of agent
  guidance requires an image rebuild — slow, and the resulting restart re-reads
  the whole session from scratch (a lot of tokens).
- **The binary is mounted as a single file.** Bind-mounting a *file* pins the
  inode, so replacing it on the host is invisible to the running container until
  it restarts.

The result is that the things most worth iterating on — the agent's behaviour
and its instructions — are the most painful to change. We want to evolve them
while the agent runs, without an image rebuild and without a container restart.

## What Changes

- **Instructions move into the binary.** What is currently split across skills
  and `CLAUDE.md` is emitted by `<role-binary> init` — the binary carries its
  role, fixed at command-tree registration. `init` takes no arguments: its
  **role is built in** (`sindri-worker init` emits worker instructions because
  that is the binary it lives in), and `mode`, the one runtime knob, comes from
  the `SINDRI_*` env the launcher sets from the index entry. The launch bootstrap
  is one stable line: *"run `sindri-worker init` and follow the instructions."*
- **Bind-mount a directory, not a file.** Install each role binary into its own
  directory and mount the *directory*; the host's atomic `mv` (already used by
  `make install`) then becomes visible to the next `exec` in the running
  container — a true hot-swap. The per-role directory holds exactly its one
  binary so nothing else leaks into the mount.
- **Install layout.**
  ```
  ~/.local/bin/sindri                          (host CLI, unmounted)
  ~/.local/share/sindri/worker/sindri-worker   → mounted into worker containers
  ~/.local/share/sindri/review/sindri-review   → mounted into reviewer containers
  ```
- **`make install` builds all and installs to those paths** via `mv` (atomic),
  which is the hot-swap action.
- **Dockerfile sheds the baked instructions.** Drop `COPY container/skills/` and
  the `CLAUDE.md` copies; add the mounted bin directory to `PATH` via `ENV`
  (retiring the build-time symlink hack). The image keeps only role-agnostic
  tooling.
- **Bootstrap is the only thing the launcher injects** — and it never changes,
  so it can stay tiny and static.

## Non-goals

- Re-prompting an already-running agent. A live binary swap updates *command
  behaviour* immediately; the instruction prose already loaded into a running
  session is frozen until the next session. (A nice follow-up is to let the
  commands the agent already calls — `issue next`, etc. — carry living guidance
  in their output, but that is out of scope here.)
- Changing roles, mounts-by-role, or the index — see `role-driven-launch` and
  `add-agent-index`.

## Impact

- Affected specs: `04-workers` (bundled tooling → mounted-dir binary + binary-
  carried instructions).
- Affected code: `Makefile` (`install` targets, build all → role dirs),
  `internal/worker/lifecycle.go` (mount the role dir; bootstrap prompt; drop
  skills symlink + `CLAUDE.md` link), `internal/worker/findAgentBin` (return the
  role dir), `container/Dockerfile` (drop skills/CLAUDE copies, add bin dir to
  PATH), `internal/agentcli` (`init` command emitting role bootstrap).
- Removed: `container/skills/*` and the baked `CLAUDE.md`/`CLAUDE.reviewer.md`
  as the source of agent instructions.
- Depends on `role-driven-launch` for the per-role binary + mode wiring.
