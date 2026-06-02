## 1. Model + state tracking

- [x] 1.1 Add `loaded bool` to `Model`; default false
- [x] 1.2 Set `m.loaded = true` at the end of the refreshMsg handler

## 2. View rendering

- [x] 2.1 `renderBacklogList` gains a `loaded bool` parameter; when false, render "Loading tasks…" in place of the empty-state line
- [x] 2.2 `renderWorkersList` gains a `loaded bool` parameter; when false, render "Loading workers…" in place of the empty-state line
- [x] 2.3 `viewList` passes `m.loaded` to both renderers

## 3. Replay engine support

- [x] 3.1 Add `Fixture.LoadingState bool`; when true, the engine leaves `m.loaded` at false so the loading frame can be captured
- [x] 3.2 When false (default), the engine sets `m.loaded = true` so existing goldens are unaffected

## 4. Goldens

- [x] 4.1 Add `list-loading` golden (LoadingState fixture, backlog view)
- [x] 4.2 Add `workers-loading` golden (LoadingState fixture, workers view via `W`)
- [x] 4.3 Wire both into `TestReplayGoldens_LoadingState`

## 5. Validation

- [x] 5.1 `openspec validate show-loading-on-startup --strict` passes
- [x] 5.2 `go build ./... && go test ./...` all green
- [x] 5.3 `sindri lint all` passes
