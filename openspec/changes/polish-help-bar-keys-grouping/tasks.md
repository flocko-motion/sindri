# Tasks

## 1. Help-bar grouping

- [ ] 1.1 Replace the flat help string in `viewList` with three " · "-separated groups
- [ ] 1.2 Order: nav (j/k, enter) · row actions (y, n) · view (r, q)

## 2. Goldens

- [ ] 2.1 Regenerate every list-view golden via `GO_UPDATE_GOLDENS=1`
- [ ] 2.2 Inspect the diff — only the help-bar string should differ

## 3. Validation

- [ ] 3.1 `openspec validate polish-help-bar-keys-grouping --strict` passes
- [ ] 3.2 `go test ./...` green; `sindri lint all` green
