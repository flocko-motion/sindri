# Tasks

## 1. Adapter

- [x] 1.1 `spec.Lookup(root, name)` returns the active Change by name (nil if archived/missing)
- [x] 1.2 `spec.Archive(root, name)` shells `openspec archive <name> --yes`
- [x] 1.3 `spec.Abandon(root, name)` removes `openspec/changes/<name>/`; refuses if missing

## 2. Decision logic

- [x] 2.1 `action.SpecAfterCloseDecision` struct (Action: None/Archive/Prompt, name, checklist counts)
- [x] 2.2 `decideSpecAfterClose` pure function — no IO
- [x] 2.3 Table-driven test covers: task missing, no spec label, spec archived, other open task remains, checklist complete, checklist incomplete, no checklist (0/0)

## 3. Action IO

- [x] 3.1 `action.MaybeArchiveLinkedSpec(root, closedTaskID)` loads board + spec, calls decision
- [x] 3.2 `action.ArchiveSpec(root, name)` (thin wrapper)
- [x] 3.3 `action.AbandonSpec(root, name)` — closes open linked tasks with reason "spec abandoned", then deletes the change folder; returns the list of closed task IDs

## 4. Orphan detection

- [x] 4.1 `Issue.SpecOrphan()` — true when task has `spec:<name>` label but `i.Spec == nil`
- [x] 4.2 `render.IssueStatus` returns `⚠ spec archived` (red bold) for SpecOrphan rows

## 5. TUI plumbing

- [x] 5.1 `mergeCompleteMsg{prID, taskID}` — mergePR returns this so the handler can chain `checkLinkedSpecCmd`
- [x] 5.2 `specCheckMsg`, `specArchivedMsg`, `specAbandonedMsg` Tea messages
- [x] 5.3 `checkLinkedSpecCmd`, `archiveSpecCmd`, `abandonSpecCmd` commands
- [x] 5.4 `handleSpecLifecycle(msg)` owns the message routing — auto-archive vs prompt vs notify
- [x] 5.5 `statusChangedMsg` with newStatus="closed" dispatches `checkLinkedSpecCmd`
- [x] 5.6 Confirm-action switch handles `abandon-spec:<name>` and `archive-spec:<name>` prefixes

## 6. TUI bindings + rendering

- [x] 6.1 `x` on a spec-only row opens the abandon confirm; on task rows it's the existing reject flow
- [x] 6.2 List view renders `m.confirmLabel` in the bottom bar when `m.confirmAction != ""`
- [x] 6.3 Confirm modal is checked before per-view handlers so y/n works from list view too

## 7. Goldens + checks

- [x] 7.1 `abandon-spec-confirm` golden captures the confirm bar on a spec row
- [x] 7.2 `go test ./...` green
- [x] 7.3 `sindri lint all` green (LOC limit, deadcode, openspec validate)
- [x] 7.4 `openspec validate sync-task-spec-lifecycle --strict` passes
