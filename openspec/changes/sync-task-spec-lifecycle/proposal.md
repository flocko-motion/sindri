# Sync td task lifecycle with openspec change lifecycle

## Why

Today the td task and the openspec change it links to (via the `spec:<name>`
label) live on two parallel, disconnected tracks. Closing or merging a
spec-linked task does nothing to the spec: the change folder stays under
`openspec/changes/`, the spec progress isn't updated, and the user has to
remember to run `openspec archive` manually. Conversely, archiving the spec
by hand leaves open linked tasks pointing at a spec that no longer exists,
without any visible drift indicator.

The whole point of the `spec:<name>` label is to pair the two — they should
move together.

## What Changes

Three connected behaviors:

1. **Auto-archive on the last close.** When a td task carrying `spec:<name>`
   closes (merge, status-pick to closed, or `td close`), the lifecycle
   checks whether any *other* open tasks still link to the same spec. If
   none remain:
   - If the spec's `tasks.md` checklist is fully ticked (or has no
     checkboxes), `openspec archive <name> --yes` runs immediately and a
     notification confirms.
   - If the checklist still has unchecked items, the TUI surfaces a
     confirm modal — "Last task for spec X closed but checklist is N/M.
     Archive anyway? (y/n)" — so silently archiving an unfinished spec
     can never happen.

2. **Abandon = both, never half.** New TUI binding: `x` on a spec-only row
   (the same key as reject on task rows; the cursor row picks the verb)
   opens a confirm modal: "Abandon spec X? Deletes the change folder and
   closes its linked open tasks. (y/n)". On `y`, every open task carrying
   `spec:X` is closed with reason "spec abandoned", then the change folder
   is removed. The "half-abandoned" state (folder gone, tasks still
   open) is intentionally not reachable.

3. **Orphan drift is visible.** A task that carries `spec:<name>` but
   whose spec is no longer an active proposal (archived externally, or
   missing) is shown with a `⚠ spec archived` warning in the status
   column — same warning style as the existing orphan-in_progress marker.
   This makes drift introduced by the openspec CLI bypassing sindri (or
   any other source) visible at a glance.

## Impact

- New capability `spec-lifecycle` captures the cross-cutting sync rules so
  they live in one place rather than scattered across action specs.
- `view-work-list` MODIFIED: `x` now overloads on spec-only rows;
  `⚠ spec archived` is a new status-cell state.
- `action-merge` MODIFIED: a successful merge of a spec-linked task may
  trigger an auto-archive or a confirm prompt; spec-side outcome is
  reported alongside the merge result.
- Adapter: `spec.Archive`, `spec.Abandon`, `spec.Lookup` added to
  `internal/adapter/spec`.
- Action: `action.MaybeArchiveLinkedSpec`, `action.ArchiveSpec`,
  `action.AbandonSpec` added to `internal/action`, with a pure
  `decideSpecAfterClose` covered by table-driven tests.
- TUI: `mergeCompleteMsg`, `specCheckMsg`, `specArchivedMsg`,
  `specAbandonedMsg` route through `handleSpecLifecycle`; new confirm
  actions `abandon-spec:<name>` and `archive-spec:<name>`.
- Goldens: new `abandon-spec-confirm` capture; existing list-view
  goldens unchanged for non-spec rows.
