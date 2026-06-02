# Tasks

- [x] 1. `render.GateLabel(g issue.Gate)` returns the canonical "☑/☐ review <type>" string
- [x] 2. `render.Gates` is rewritten as a thin loop over `GateLabel`
- [x] 3. `cmd/sindri/pr.go` `pr review <id>` calls `render.GateLabel` instead of `g.Name`
- [x] 4. `TestGateLabel` pins the format for review-code and review-security (approved + unapproved)
- [x] 5. `go test ./...` + `sindri lint all` + `openspec validate unify-gate-label-format --strict` pass
