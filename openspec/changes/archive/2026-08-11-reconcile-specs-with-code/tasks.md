# Tasks

Section 3 lands after `separate-hub-from-frontends`, so the layout is described once and
correctly rather than twice.

## 1. Archive what is already done

- [ ] 1.1 Archive `add-repo-registry`, `add-coauthor-role`, `add-planner-role` and
      `add-host-pr-approve` — all four are implemented with their task lists complete. This
      is what removes the superseded text from the base specs, including `04-workers`' claim
      that the review agent takes no dwarf name.
- [ ] 1.2 Confirm after archiving that the base specs carry the corrected requirements and
      that `openspec validate --all` still passes.

## 2. Confirm the restated specs against the code

- [ ] 2.1 Check the task-model requirement against `internal/hub/store/owned.go`,
      `internal/hub/workflow/ownedsource.go` and `internal/hub/workflow/importtd.go`:
      sindri's own tasks as a source, `OwnedPrefix = "td-"`, owned writes landing in the
      hub's store, mirrored writes going through their source's tool, and mirrored-only
      refresh.
- [ ] 2.2 Check the td requirement against `internal/adapter/tasks/td/sqlite.go` — read-only,
      once, no CLI invocation.
- [ ] 2.3 Check the memory and stats requirements against `internal/hub/agent/memory.go` and
      the stats payload, and the pane-reporting requirement against
      `internal/adapter/herdr` and `internal/ui/attach/herdr.go`.

## 3. Correct the prose sections (after the separation lands)

- [ ] 3.1 Rewrite `01-architecture`'s Source layout: it names `internal/issue`,
      `internal/board`, `internal/render`, `internal/ghlocal/store`, `internal/worker`,
      `internal/agentcli` and `cmd/sindri-review`, none of which exist. Describe the tree as
      it is once the separation has landed.
- [ ] 3.2 Rewrite the Structure sections of `03-gh-local` and `04-workers`, which name the
      same absent packages.
- [ ] 3.3 Correct `ARCHITECTURE.md`'s layout and topology lines in the same pass (its rule
      wording is corrected earlier, by `name-the-port-not-the-tool`).

## 4. Correct the README

- [ ] 4.1 The hub is global, not "a single per-repo hub": fix the opening description and the
      diagram.
- [ ] 4.2 State and layout: hub state is central under the state directory
      (`tools/paths.StateDir()`), not `.sindri/hub.db` in the repo, and sindri owns tasks —
      the table's "Tasks (source of truth) | td" row is wrong.
- [ ] 4.3 Bring the command reference up to date: roughly twenty shipped commands are absent
      from it, including `agent stats/memory/rebase/rebuild/restart/dir/plan`, the `meeting`
      group, the `repo` group, `task comment/refresh/close/delete`, `pr scrap` and
      `hub restart/logs/status/stop`.

## 5. Verify

- [ ] 5.1 `openspec validate --all` passes.
- [ ] 5.2 Read the four corrected documents end to end for claims that are still false — this
      change is worth nothing if it leaves a second stale sentence behind.
