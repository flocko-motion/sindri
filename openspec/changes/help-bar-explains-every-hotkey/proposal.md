# Top help-bar lists `e:edit`

## Why

The `e` hotkey shipped with no entry in the work-list top bar, so a
user looking at the screen has no way to discover it. Reviewer:

> hotkey must be explained in top bar

Bake this in as a project rule: every action binding the list view
actually accepts SHALL be listed in the top help bar, so the bar stays
the source of truth for what's available.

## What Changes

- The help string in `viewList` now reads
  `j/k:nav  enter:open  y:copy  n:new  e:edit  r:refresh  q:quit` —
  the `e:edit` entry was missing.
- `view-work-list` MODIFIED: an "Identical across interfaces"-style
  requirement is added so the work list's help bar SHALL list every
  binding accepted by the list view, in keystroke form.

## Impact

- Affected spec: `view-work-list`.
- Affected code: `internal/tui/backlog.go` (one-line help string).
- Goldens regenerated for every list-view capture (string change in
  the top bar).
