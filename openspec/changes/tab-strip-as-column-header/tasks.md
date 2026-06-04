# Tasks

- [x] 1. `tabHeader(active, tasksActive, workersActive)` returns the styled tab strip — active bold+highlight, inactive dim, both with hotkey brackets
- [x] 2. `viewList` removes the top-right view selector and prepends the tab strip to the column content
- [x] 3. The old green column header (`"Tasks"`/`"Workers"`) is gone — `renderColumn` is called with an empty header
- [x] 4. `view-work-list` adds a "tab strip is the column header" requirement with scenarios for backlog active vs workers active
- [x] 5. Goldens regenerated; visually inspected list-default and workers
- [x] 6. `go test ./...` + `sindri lint all` + `openspec validate tab-strip-as-column-header --strict` pass
