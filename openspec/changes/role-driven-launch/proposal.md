# Role-driven agent launch

## Why

The reviewer is special-cased throughout the launch path even though it is just
an agent in a different *role*:

- `StartReviewer` duplicates most of `Start` (mounts, env, claude home, startup
  script) with a few diverging lines.
- The reviewer is tagged with a magic `sindri.worker=_reviewer` label, and
  `List()` forces "the main repo is the reviewer" by position.
- The only real differences are two axes — the **command subset** (worker:
  `issue next`/`submit`/`done`/`pr create`; reviewer: `issue view`/`pr approve`/
  `reject`) and the **workspace topology** (worker: own worktree `:rw` + repo
  `:ro`; reviewer: whole repo `:ro`) — plus knobs (env, bootstrap mode, naming).

Role should be a *parameter* that selects those differences, driven by the agent
index, not duplicated code plus position guessing.

## What Changes

- **`RoleSpec`** — a single description of what a role implies: which binary,
  the mount topology, whether a base branch is needed (`GH_LOCAL_BASE`), and the
  bootstrap mode. `worker` and `reviewer` are two `RoleSpec` values.
- **One launch path.** `Start(projectRoot, name, role, opts)` switches on
  `RoleSpec`; `StartReviewer` is removed. Role comes from the agent's index
  entry (see `add-agent-index`), not from position or the `_reviewer` label.
- **Two binaries kept, deliberately.** `sindri-worker` and `sindri-review`
  remain separate binaries for hard capability isolation: the reviewer binary
  does not register mutating commands, so they cannot be invoked, and its
  workspace is mounted `:ro`. Capability (binary) and authority (mount) agree —
  defense in depth.
- **Shared source, divergent surface.** Both binaries build from
  `internal/agentcli`; the boundary is *which subset each `*Root()` registers*.
  The rule to protect: never register a mutating command on `ReviewRoot()`.

## Non-goals

- Collapsing the two binaries into one role-gated binary. Considered and
  rejected — separate binaries enforce the capability boundary structurally
  rather than via a runtime `if role ==` check.
- Moving instructions into the binary or changing how binaries are mounted —
  that is `hot-swap-agent-tooling`.

## Impact

- Affected specs: `04-workers` (role-driven mounts; capability isolation by
  role binary), `view-workers` (reviewer distinct as a role, not by position).
- Affected code: `internal/worker/lifecycle.go` (merge `Start`/`StartReviewer`
  behind `RoleSpec`), `internal/worker/worker.go` (read role from index),
  `cmd/sindri/main.go` (`worker review` becomes role-parameterised start).
- Depends on `add-agent-index` for the `role` field to read from.
