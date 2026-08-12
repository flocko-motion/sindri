# Mint sindri's own task ids as `sd-`

## Why

Sindri's own tasks are minted `td-`, a prefix inherited from the tool its store replaced.
The spec already has to explain the borrowed name — "the prefix kept for continuity with
the tool sindri's own store replaced" — which is a note about history, not about ownership.
A task sindri owns should say so.

The prefix is only the visible half. The reason it was worth doing is that nothing owned
the question "what does this id mean?": the knowledge sat in five places, two of which had
already drifted to a hardcoded `"td-"` that no longer followed the constant. That is fixed
first, in `internal/hub/task`, and the prefix change is then one line.

## What changes

- New sindri-owned tasks get an `sd-` id.
- `td-` becomes the recognised LEGACY form of the same ownership. Nothing is rewritten: a
  task id is embedded in its PR id (`pr-td-abc123`), its git branch name and its agent's
  worktree path, so renaming one would orphan a live branch, a PR record and possibly a
  running agent workspace. Existing tasks keep their ids and keep working until they close.
- `os-` and `gh-` are unchanged.

## Impact

- Specs: `hub` (the prefix enumeration and the scenario naming the owner), `03-gh-local`
  (the branch-name example).
- Code: `internal/hub/task/ids.go` owns the scheme; the hub, the workflow and the task
  adapters ask it instead of comparing prefixes themselves.
- The td import asks specifically whether a row is a legacy `td-` id, which is a different
  question from what sindri mints — conflating them would make the import silently carry
  nothing.
