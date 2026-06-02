# Tasks

- [x] 1. `spec.Title(projectRoot, name)` returns the first H1 of proposal.md, falling back to the slug
- [x] 2. `newCreateTaskModel` pre-fills the title input via `spec.Title` when invoked with a non-empty spec name
- [x] 3. `view-work-list` scenario added: "n on a spec-only row pre-fills the title from the spec proposal"
- [x] 4. `go test ./...` + `sindri lint all` pass; `openspec validate prefill-title-from-spec-proposal --strict` passes
