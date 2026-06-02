# Tasks

## 1. Adapter

- [x] 1.1 `td.Detail(root, id)` returns `(description, acceptance string, err error)` from `td show --json`

## 2. TUI data seams

- [x] 2.1 `fetchTaskDetail` rerouted through `td.Detail` so it returns description body only
- [x] 2.2 New `fetchTaskAcceptance` seam + production fetcher

## 3. Detail-state split

- [x] 3.1 `detailState` grows `leftCol` and `rightCol` fields; `content` retained for PR/worker single-column path
- [x] 3.2 `issueDetail` builds left (metadata, gates, worker, PRs) and right (description, acceptance, comments) from the structured fetch
- [x] 3.3 `specDetail` uses `leftCol` for the spec metadata (right pane empty since there's no task body)

## 4. Layout

- [x] 4.1 `viewDetail` lays out two columns when `leftCol != ""`, else falls back to single-column for PR/worker
- [x] 4.2 `resizeViewports` sizes `vpDetail.Width = m.width/2 - 4`
- [x] 4.3 `detailColWidth()` is the per-column width passed to `issueDetail` so section borders fit the viewport

## 5. Spec + goldens

- [x] 5.1 `view-item-detail` "Task detail" requirement rewritten for two columns + structured fields
- [x] 5.2 Goldens regenerated; visually inspected `detail-task`
- [x] 5.3 `go test ./...` + `sindri lint all` + `openspec validate two-column-task-detail --strict` pass
