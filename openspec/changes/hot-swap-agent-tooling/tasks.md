# Tasks

## 1. `init` carries the instructions

- [ ] 1.1 Add an `init` command to `internal/agentcli`, role-bound at
      registration (`WorkerRoot` → worker bootstrap; `ReviewRoot` → reviewer)
- [ ] 1.2 Port the skill + CLAUDE content into `init` output, on two axes:
      `sindri-worker init` emits worker instructions and branches on the worker
      `mode` (next = autonomous, choose = collaborative) read from env;
      `sindri-review init` emits the reviewer instructions (single behavior)
- [ ] 1.3 Launch bootstrap becomes the single line "run `<binary> init` and
      follow the instructions"; mode is passed via env from the index entry

## 2. Install layout + hot-swap

- [ ] 2.1 `make install`: build all binaries, `mv sindri` → `~/.local/bin`,
      `mv sindri-worker` → `~/.local/share/sindri/worker/`,
      `mv sindri-review` → `~/.local/share/sindri/review/`
- [ ] 2.2 `findAgentBin` returns the per-role *directory* to mount
- [ ] 2.3 `lifecycle.go`: bind-mount the role directory (not the file) and ensure
      it is on `PATH` in the container

## 3. Shed baked instructions

- [ ] 3.1 Dockerfile: remove `COPY container/skills/`, `CLAUDE.md`,
      `CLAUDE.reviewer.md`; add the mounted bin dir to `PATH` via `ENV`; drop the
      build-time `sindri-worker`/`sindri-review` symlinks
- [ ] 3.2 `lifecycle.go`: remove the skills symlink and `CLAUDE.md` link from the
      container startup script
- [ ] 3.3 Delete `container/skills/` and the baked CLAUDE files

## 4. Validation

- [ ] 4.1 `openspec validate hot-swap-agent-tooling --strict` passes
- [ ] 4.2 `go test ./...` green; `sindri lint all` green
- [ ] 4.3 Manual hot-swap: with a worker running, `make install` a changed
      `sindri-worker`; the next subcommand in the container runs the new binary
      with no restart and no image rebuild
