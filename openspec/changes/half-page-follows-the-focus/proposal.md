# The half-page keys follow the focus, as the line keys do

## Why

`ctrl+d`/`ctrl+u` decided what to scroll from the active tab: on the PRs tab they always half-paged
the diff, and everywhere else they always moved the list cursor. `j`/`k` and `J`/`K` decide from the
focus, and one of them goes through a resolver written for exactly this question. So focusing the
detail column and pressing `ctrl+d` moved the list — the two scroll speeds disagreed about what they
scrolled.

The tab is the wrong thing to ask. It says which view is open, never which half of it the user is
looking at, and the answer was already available one function away.

## What changes

- `ctrl+d`/`ctrl+u` resolve their target by focus: the list when the list has it, and otherwise the
  viewport `scrollTarget()` returns — the detail pane, or the PRs tab's metadata column.
- Half-paging a viewport moves it by half its own height, in `halfPage`, beside the resolver rather
  than inline in two key cases that were free to disagree.
- The keymap comment describing the old tab-based rule is replaced. It named the special case as
  fact, which is how the previous reader came to trust it.

## Impact

- Specs: `view-tui` (what the half-page keys move).
- Code: `internal/ui/tui/onkey.go`, `internal/ui/tui/viewport.go`, `internal/ui/tui/keys.go`.
- On the PRs tab with the list focused, `ctrl+d` now moves the PR list rather than the diff, which
  is what focus-first means there: the diff keeps `J`/`K`, and `ctrl+d` reaches it through the
  focused column like every other pane. Worth a second look if reading long diffs turns out to want
  a half-page of its own — that would be a key for the diff, not a fifth rule for these two.
- `TestPRsDetailHalfPages` pinned the tab-based behaviour and is replaced by one that pins the
  focus-based rule.
