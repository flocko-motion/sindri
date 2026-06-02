# Tasks

## 1. Spec row alignment

- [x] 1.1 `prioColCells = 2` and `tsColCells = 14` constants in `backlog.go`
- [x] 1.2 `buildBacklogRows` pads priority + timestamp cells with `padCell` so empty values on spec rows still occupy the column
- [x] 1.3 Spec row's status, title columns line up with the same columns on task rows under the SimpleFixture golden

## 2. Spec glyph

- [x] 2.1 `render.IssueStatus` renders spec marker as 📄 (single-codepoint, 2-cell, no VS16)
- [x] 2.2 `issue.Title` renders the spec prefix on linked tasks as 📄 to match

## 3. Pre-link new-task creation

- [x] 3.1 `Model.cursorSpecName()` returns the spec name when the cursor sits on a spec-only row, else ""
- [x] 3.2 `newCreateTaskModel` takes a `specName` argument and stores it on `createTaskModel`
- [x] 3.3 `keys.NewTask` handler passes `m.cursorSpecName()` to `newCreateTaskModel` — `n` from any non-spec row is unchanged
- [x] 3.4 On submit, when `specName != ""`, the created task carries the `spec:<name>` label alongside any other labels (e.g. `require-review-code`)
- [x] 3.5 Modal view shows "Linked to spec: 📄 <name>" at the top when `specName != ""`

## 4. Replay + goldens

- [x] 4.1 Script step: cursor moves up to the spec row (`auth-refactor`), press `n`, capture `create-spec-linked`
- [x] 4.2 Capture asserted by `TestReplayGoldens`
- [x] 4.3 Existing goldens regenerated (alignment + glyph change visible)

## 5. Validation

- [x] 5.1 `openspec validate align-spec-rows-and-link-create --strict` passes
- [x] 5.2 `go build ./... && go test ./...` all green
- [x] 5.3 `sindri lint all` passes
