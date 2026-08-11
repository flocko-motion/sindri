# Tasks

Ordered. Section 1 lands on its own and merges before the rest starts — every later
diff would otherwise carry it. Sections 2-5 are one standing branch with checkpoints
between them, because they rewrite the same import blocks in the same files.

## 1. Preparation: the exchange package (merge before section 2 starts)

- [ ] 1.1 Create `internal/api` and move the 17 wire types out of `internal/hub`
      (7 views + 10 request types), plus the 8 types `wiring.go` aliases from
      `workflow`/`project`/`agentchan`/`task`/`agent`.
- [ ] 1.2 Move the 8 wire-bearing row types out of `internal/hub/store` — `Task`,
      `PR`, `Project`, `ChatMessage`, `ChatMember`, `Comment`, `Event`, `Review` —
      and have `store` import `internal/api`. `AgentState`, `OwnedTask`, `Agent`,
      `Store` and `ProjectStore` stay: they never cross the wire.
- [ ] 1.3 Split `internal/config`: `Config`, `GitHub` and `Lint` move to
      `internal/api`; `Load`, `Write`, `validate` and `Abs` stay and import them.
- [ ] 1.4 Give `internal/api` the resolved section type `{Key, Title string; Count
      int}`, and leave the `Count func(Board) int` registry in `internal/hub/commands`
      — the hub resolves counts against `BoardState` and only the number crosses.
- [ ] 1.5 Move the pure functions over the exchange types into `internal/api`: task
      arrangement (`ArrangeTasks`, `Descendants`) and the open/done predicates.
- [ ] 1.6 Move `internal/hub/client` to `internal/client` (it has no hub-side owner:
      `cmd/sindri-worker`, `internal/ui/cli` and `internal/ui/tui` use it).
- [ ] 1.7 Add a test asserting `internal/api` imports nothing outside the standard
      library — the invariant the whole change rests on.

## 2. The hub as its own binary

- [ ] 2.1 Add `cmd/sindri-hub`: a thin entrypoint that constructs and runs the hub,
      resolving nothing else. Move `agent.SyncPodBin()` (today
      `internal/ui/cli/hub.go:183`) into its startup.
- [ ] 2.2 Make `sindri hub start` exec it: `syscall.Exec` in the foreground so
      signals reach the hub with no wrapper process, and the existing detached spawn
      (`ui/cli/hubclient.go:117-134`) for `--bg`. Resolve the binary beside `argv[0]`
      before `PATH`.
- [ ] 2.3 Delete `hub.New()` from `internal/ui/cli/hub.go` and the hub-lifecycle
      logic that only existed to support it.
- [ ] 2.4 Add the binary to `Makefile`, `scripts/install.sh`, the release tarball and
      the README install section, each of which enumerates what ships.
- [ ] 2.5 Confirm a version-skewed pair is still reported: the pid file already
      stamps the hub's version (`internal/hub/server/pidfile.go`) and the client
      already warns on mismatch (`ui/cli/hubclient.go:50`).

## 3. Nothing under internal/ui imports internal/hub

- [ ] 3.1 Repoint every `internal/hub` and `internal/hub/store` import in
      `internal/ui` at `internal/api` and `internal/client` (24 and 12 files).
- [ ] 3.2 Rehome the residue the front-ends legitimately need: `Container(root,
      name)` to `internal/container` (both already import it), `SocketPath` and the
      pid/liveness helpers to `internal/tools/paths` and the client, `RepoTag` to
      `internal/api` as the protocol's project key.
- [ ] 3.3 Add the import-guard test: walk the import graph and fail if any package
      under `internal/ui` imports `internal/hub/…`. `make verify` runs
      `go test ./...`, so this gates every build and CI run.
- [ ] 3.4 Delete the re-export block in `internal/hub/wiring.go` (the `agentDeps`/
      `workflowDeps`/`chatDelivery` seam adapters stay — they are the reason the
      hub's modules need not import the hub).
- [ ] 3.5 Milestone PR here: this is where the invariant first holds.

## 4. Presentation leaves the API

- [ ] 4.1 Move the display words to `internal/ui/theme`: `PriorityLabel`,
      `PriorityCode`, `PriorityWords`, `StateLabel`, `FormatClients`.
- [ ] 4.2 Move the chat glyphs and help text out of `internal/hub/chat/service.go:44-77`
      (`HelpText`, `UserIcon`, `AgentIcon`, `SystemIcon`, `Icon`) into
      `internal/ui/theme`, so `ui/theme` no longer imports `internal/hub/chat`. The
      hub keeps composing the room's history, but from data, not glyphs.
- [ ] 4.3 Keep the stored colour index in the hub as a per-machine preference, and
      move the palette and the index→colour mapping to `internal/ui/theme`.
- [ ] 4.4 Drop `TaskRow.Last` from the wire if the front-ends can derive it while
      walking the arranged rows; keep `Depth`, `PR` and `PRKind`, which are data.

## 5. The front-ends stop reaching past the API

- [ ] 5.1 `ui/tui/startup.go:53`: the openspec-availability notice comes from the
      hub, which is the process that needs the tool, instead of the TUI probing its
      own PATH through the spec adapter.
- [ ] 5.2 `ui/tui/nested.go:39`: take liveness from the client rather than importing
      `internal/hub/server`.
- [ ] 5.3 `ui/cli/hublist.go:76`: report hub uptime from the board instead of
      shelling out to `ps`.
- [ ] 5.4 Confirm `internal/ui/cli/shadow.go`'s PATH probe is the one remaining
      deliberate exception (it exists to detect a second installed copy) and say so
      in its header.

## 6. Verify

- [ ] 6.1 `make verify` passes: build, `go test ./...` (including the two new
      invariant tests), and `brokkr lint`.
- [ ] 6.2 `openspec validate --all` passes (run by the submit lint gate).
- [ ] 6.3 Manual: `sindri hub start` in the foreground and with `--bg`; `sindri tui`
      with no hub running still auto-starts one; a coauthor and a worker both still
      launch, attach and submit.
- [ ] 6.4 Confirm the front-end binary no longer links sqlite or the adapters
      (`go list -deps ./cmd/sindri` mentions neither `modernc.org/sqlite` nor
      `internal/adapter/git`).
