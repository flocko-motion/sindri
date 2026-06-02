# Pre-fill the new-task title from the spec proposal

## Why

Pressing `n` on a spec-only row opens the new-task modal pre-linked to
that spec — but the title field is empty. The user immediately retypes
something close to the spec name. The modal should hand them that
starter text.

## What Changes

- `spec.Title(projectRoot, name)` reads `openspec/changes/<name>/proposal.md`
  and returns the first level-1 heading. When the file is missing or
  has no H1, it falls back to the slug.
- `newCreateTaskModel`, when invoked with a non-empty spec name,
  pre-fills the title input with `spec.Title(...)`. The user can edit
  before submit.

## Impact

- Affected spec: `view-work-list` — the existing "Pre-link new-task
  creation from a spec row" requirement gains a scenario for the title
  pre-fill.
- Affected code: `internal/adapter/spec/spec.go`,
  `internal/tui/create_task.go`.
- Golden `create-spec-linked` regenerated: title now shows the spec
  slug (the fixture has no proposal.md). Live specs with proposal.md
  show their actual H1.
