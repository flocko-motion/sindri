# Tasks

## 1. RoleSpec

- [ ] 1.1 Define `RoleSpec` in `internal/worker`: binary, mount topology
      (workspace + repo mounts and their rw/ro modes), base-branch env flag,
      bootstrap mode
- [ ] 1.2 Two values: `worker` (worktree `:rw` + repo `:ro`, `GH_LOCAL_BASE`;
      modes next|choose) and `reviewer` (repo `:ro` as workspace, no base; one
      mode today). Mode fine-tunes behaviour within a role; the binary owns the role

## 2. Unify launch

- [ ] 2.1 Refactor `Start` to take a role and build podman args from `RoleSpec`
- [ ] 2.2 Remove `StartReviewer`; route `worker review` through the unified path
- [ ] 2.3 Drop the `sindri.worker=_reviewer` magic label; label with the agent's
      real name + role
- [ ] 2.4 Read role from the agent index entry instead of inferring by position

## 3. Capability isolation (verify, don't weaken)

- [ ] 3.1 Confirm `ReviewRoot()` registers no mutating commands
      (`submit`/`done`/`pr create`); add a test asserting the reviewer command
      tree excludes them
- [ ] 3.2 Confirm reviewer workspace mount remains `:ro`

## 4. Validation

- [ ] 4.1 `openspec validate role-driven-launch --strict` passes
- [ ] 4.2 `go test ./...` green; `sindri lint all` green
- [ ] 4.3 Manual: launch a worker and the reviewer via the unified path; both
      get correct mounts, env, and bootstrap
