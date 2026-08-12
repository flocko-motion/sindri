# Tasks

Section 1 lands first and alone: it is the document the reviewer is briefed with on
every verdict, so correcting it is what makes the rest of this work reviewable against
the right rule.

## 1. Correct the rule in ARCHITECTURE.md (land first)

- [ ] 1.1 Reword "The core calls adapters" to name the port: the core depends on the
      abstraction where a family of implementations exists, a composition root chooses
      the implementation, and a single-implementation tool (git, tmux) is imported
      directly by rule.
- [ ] 1.2 State the front-end boundary in the same terms — the front-ends speak to the
      hub through the exchange format and the client, and link no core code — so a
      reviewer can check it without reading the openspec deltas.
- [ ] 1.3 Fix the paths the document names: `internal/tui` is `internal/ui/tui`, and
      `internal/client` does not exist yet (it arrives with
      `separate-hub-from-frontends`; until then the client is `internal/hub/client`).
- [ ] 1.4 Leave the layout and topology sections to the change that alters them, so
      this edit describes only what is true today plus the rules.

## 2. Close the three reach-arounds

- [ ] 2.1 `hub/workflow/engine.go:23-24` — inject the task-source set from the
      composition root instead of constructing `spec.Source{}` and `github.Source{}`
      in the engine. The engine keeps `[]tasks.Source` and learns nothing about who
      implements it.
- [ ] 2.2 `hub/comments/service.go:84,87,150,156` — reach comment read/write through
      the task-source port rather than `github.Number`/`AddComment`/`IssueComments`, so
      a second tracker is an adapter and not an edit to the core.
- [ ] 2.3 `hub/workflow/pr.go:278` — put `spec.Validate(wt)` behind the quality-gate
      abstraction the submit path already needs, alongside the lint gate.
- [ ] 2.4 Leave `hub/workflow/importtd.go` naming td: it is a one-way migration *from*
      td, and naming what is being migrated from is honest. Say so in its header.

## 3. Adapters for the tools the core shells out to

- [ ] 3.1 `hub/server/pidfile.go:83,96` — `ps` and `lsof` behind a process adapter.
- [ ] 3.2 `hub/repo/repo.go:52` — the `brokkr` binary behind a lint-gate adapter (the
      same seam task 2.3 needs).
- [ ] 3.3 `update/update.go:209,221` — route `gh` through
      `internal/adapter/tasks/github`, which already wraps it, instead of a second call
      site for the same tool.
- [ ] 3.4 `hub/agent/binaries.go:28,45,64` — keep the `exec.LookPath` probes but state
      in the header why resolving a binary path is not a tool invocation, or move them
      behind the process adapter with the rest.

## 4. The core prints nothing

- [ ] 4.1 `hub/agent/lifecycle.go:147` — record the reopen-on-delete warning where the
      interfaces can render it, instead of `fmt.Printf` to the hub's stdout.

## 5. The layer vocabulary is a closed set

- [ ] 5.1 Retype the four `persistence` files in `internal/hub/store` as `adapter`, and
      the two `logic` files there with them — one package, one type.
- [ ] 5.2 Retype the six one-offs: `cmd/sindri/runtime.go` (`composition root` →
      `assembly`), `internal/ui/attach/herdr.go` (`application helper` → `assembly`),
      `internal/hub/agent/names.go` (`headless helper` → `logic`),
      `internal/ui/tui/util.go` (`small shared helpers` → `ui`),
      `internal/ui/tui/screenshot.go` (`dev/test harness` → `ui`),
      `internal/brokkr/codemap/codemap.go` (`dev tool` → `logic`).
- [ ] 5.3 Retype `internal/ui/theme/theme.go` as `rendering` and
      `internal/hub/task/view.go`'s successor accordingly — the two types the
      vocabulary has and nothing uses.
- [ ] 5.4 Retype the two `logic` files inside `internal/ui/cli` (`hubclient.go`,
      `shadow.go`): a front-end package holds no logic, so either the file is `command`
      or its content belongs elsewhere.
- [ ] 5.5 Add a test asserting every non-test file's `type:` is one of the seven, so
      the vocabulary cannot drift one file at a time.

## 6. The limits convention

- [ ] 6.1 Add a neighbour pointer where one is genuinely owed — a `limits:` line that
      excludes a concern another package owns and does not name it. No sweep of the 112
      files whose exclusions have no owner.

## 7. Verify

- [ ] 7.1 `make verify` passes, including the new vocabulary test.
- [ ] 7.2 `openspec validate --all` passes.
- [ ] 7.3 Confirm no core package imports a concrete adapter where a port exists:
      `internal/adapter/tasks/{github,spec}` appear only in the composition root and in
      `importtd.go`.
