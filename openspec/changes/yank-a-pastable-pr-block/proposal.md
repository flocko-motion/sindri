# Yanking a PR gives you something you can paste

## Why

From the PRs list, `y` copied the PR's id and nothing else. An id alone does not say who is
working on it, what it is for, or where the work sits — so pasting it into a message or a ticket
means retyping all of that from the screen, including the worktree path, which is the one field
nobody retypes from memory.

The path was the specific gap. It is already computed for the interactive detail column
(`m.agentWorkspacePath`), and it appeared in neither line builder — because two separate places
described the same PR and had drifted apart.

## What changes

- From the PRs list, `y` copies a block identifying the PR: its id and status, its author, its
  task's id and title, its branch, and its author's worktree path.
- In the detail pane, `y` is unchanged: it copies the one focused item's value. That is how a
  single field is lifted out on its own, and the list block does not take it over.
- On every other tab's list, `y` still copies the id.
- `Y` is untouched; the ENTER modal still shows the full detail, diff included.
- The block and the full detail both come from `prIdentity`, so a field added to one cannot go
  missing from the other. That drift is what lost the path in the first place.
- Where the detail has not yet arrived for the selected PR, `y` copies the id rather than the
  block. The detail is fetched lazily, so pressing `y` straight after moving the selection would
  otherwise paste the previous PR's fields under this PR's name.

## Impact

- Specs: `view-tui` gains a requirement for what yanking copies, which was unspecified.
- Code: `internal/ui/tui/tab_prs.go` (the shared builder and the block) and
  `internal/ui/tui/onkey.go` (the list branch of `y`).
- No new key and no key rebound: this narrows what one existing key copies in one place, and
  nothing becomes unreachable — the full detail is still the ENTER modal's, and `sindri show`
  still prints a PR whole.
- Discoverability needs nothing here: the footer already carries `y/Y — yank/all`
  (`keys.go`), and that label still describes the keys after this change. The task's note that
  `Y` is unadvertised no longer holds.
