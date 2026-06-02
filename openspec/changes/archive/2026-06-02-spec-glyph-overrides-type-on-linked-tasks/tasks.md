# Tasks

## 1. Type-column rule

- [x] 1.1 `render.TypeColumn` returns 📄 when `iss.HasSpec()` is true (covers spec-only AND linked task rows)
- [x] 1.2 Plain (unlinked) tasks still return their type glyph
- [x] 1.3 Orphan-linked tasks (spec gone) fall back to the type glyph

## 2. Title prefix dedup

- [x] 2.1 `issue.Title` drops the "📄 " prefix on linked tasks; the title becomes `<spec> · <task>`

## 3. Spec + goldens

- [x] 3.1 `view-work-list` "Items as rows" updated for linked-task scenario
- [x] 3.2 `openspec validate spec-glyph-overrides-type-on-linked-tasks --strict` passes
- [x] 3.3 List-view goldens regenerated; manually inspected list-default
- [x] 3.4 `go test ./...` and `sindri lint all` pass
