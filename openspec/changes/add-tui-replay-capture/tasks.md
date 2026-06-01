## 1. DSL and key mapping

- [x] 1.1 Define the DSL grammar (literal runes, named special keys, parenthesized directives) in a short doc comment in `internal/tui/replay.go`
- [x] 1.2 Implement a token → `tea.KeyMsg` mapper covering at least: `down`, `up`, `left`, `right`, `enter`, `esc`, `tab`, `space`, `ctrl+<letter>`
- [x] 1.3 Make unknown tokens a hard parse error with a message naming the offending token

## 2. Replay engine

- [x] 2.1 Implement `tui.Replay(script string, fx Fixture, captureDir string) error` that drives `Model.Update` and reads `Model.View()`
- [x] 2.2 After each `Update`, drain returned `tea.Cmd` values by running them and feeding their messages back, recursing until no commands are pending (per-cmd timeout drops `tea.Tick`/blinks)
- [x] 2.3 Implement `(resize W H)`, `(drain)`, and `(capture <name>)` directives (`resize WxH` form also accepted)
- [x] 2.4 Accept `(sleep N)` as an alias for `(drain)` so existing mental models work; no wall-clock time is consumed
- [x] 2.5 Force `lipgloss.SetColorProfile(termenv.TrueColor)` at engine entry so captures contain real ANSI

## 3. Fixtures

- [x] 3.1 Provide a `tui.Fixture` type holding `[]issue.Issue`, `[]worker.Worker`, plus initial width/height
- [x] 3.2 Add `SimpleFixture()` covering spec-only, open, in_progress (with worker + PR + met gate), and closed; workers include a running dwarf, an idle dwarf, and the reviewer
- [x] 3.3 Document how to add a new fixture (package doc on `replay_fixtures.go`)

## 4. Frame capture

- [x] 4.1 At each `(capture <name>)`, write `<captureDir>/<name>.ansi` with the raw View output and `<captureDir>/<name>.txt` with ANSI escapes stripped
- [x] 4.2 Provide a small `tui.AssertGolden(t, captureDir, name)` helper for committed-golden comparisons
- [x] 4.3 Support a `-update` mechanism via `GO_UPDATE_GOLDENS=1` so intentional changes can rewrite the goldens in one pass

## 5. Golden tests for the freshly-touched states

- [x] 5.1 Capture the work list under each filter (open, all, closed) — exercises the new `f` key (`TestReplay_BasicListAndFilter` + golden `list-default`/`list-all`/`list-closed`)
- [x] 5.2 Capture the item detail of a task with PRs, gates, worker, and a `spec:` link (golden `detail-task`)
- [x] 5.3 Capture the item detail of a spec-only Issue (golden `detail-spec`)
- [x] 5.4 Capture the workers view, role column included (golden `workers`)
- [x] 5.5 Capture the merge-confirm modal and its prompt text (golden `merge-confirm`)
- [x] 5.6 Capture the reject-reason input bar with placeholder (golden `reject-reason`)
- [x] 5.7 Wire the goldens into a single `TestReplayGoldens` so a layout regression fails the package test

## 6. Optional: sindri tui --script flag

- [x] 6.1 Add `--script <file>` and `--capture-dir <dir>` flags to `cmd/sindri/tui.go`, dispatching to `tui.Replay`
- [x] 6.2 Document the flag and the DSL in `sindri tui --help`

## 7. Validation

- [x] 7.1 `openspec validate add-tui-replay-capture --strict` passes
- [x] 7.2 `go build ./... && go vet ./... && go test ./...` all green
- [x] 7.3 `sindri lint all` (loc + deadcode + openspec) passes
