# Tasks

- [x] 1. `createTaskModel.descInput` is a `textarea.Model` (was `textinput.Model`)
- [x] 2. `newCreateTaskInputs` is the shared builder used by both `newCreateTaskModel` and `newEditTaskModel`; modal width 80, textarea width ~67, height 5 rows
- [x] 3. `Desc:` label sits on its own line above the textarea so wrapped rows align
- [x] 4. `ctrl+s` is bound to submit on every field; plain `enter` submits only when activeField != fieldDesc, otherwise it falls through to the textarea (newline)
- [x] 5. `trySubmit` extracted so both submit paths share the same validation
- [x] 6. Bottom help line spells out the dual `enter` behavior and the new `ctrl+s` shortcut
- [x] 7. `action-create-task` spec updated: multi-line description + submit shortcut requirements
- [x] 8. `go test ./...` + `sindri lint all` + `openspec validate new-task-description-textarea --strict` pass
- [x] 9. Goldens regenerated (create-spec-linked, edit-task)
