# Group the help-bar key hints by purpose

## Why

The top-right help bar in the list view currently reads as a flat sequence:

```
j/k:nav  enter:open  y:copy  n:new  r:refresh  q:quit
```

That works, but the eye can't tell which keys belong together — the cursor
moves (j/k), the row opens (enter), the row is copied (y), one creates a new
item (n), the whole view refreshes (r), and quit ends the session. Mixed
together, every glance is a re-read.

This change is a small mock so the new TUI flow (a spec-only row + pressing
`n` on it to create a linked task) has something to act on. It also captures
a real, tiny piece of polish so it's not wasted work if you decide to accept it.

## What Changes

- Group help-bar entries into three logical chunks separated by " · ":
  navigation, row actions, view-level actions.
  ```
  j/k:nav enter:open · y:copy n:new · r:refresh q:quit
  ```
- Keep the binding strings identical — only the separator and order change.

## Impact

- Affected specs: `view-work-list` — the "Identical across interfaces"
  requirement that touches the help bar is unaffected; this only changes
  the cosmetic grouping.
- Affected code: `internal/tui/backlog.go` (the `help := dimStyle.Render(...)`
  line inside `viewList`).
- Goldens: every list-view golden picks up the new help-bar string, so
  they need a regenerate pass.
