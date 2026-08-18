# Tasks

## 1. Unread mail in the agent detail jumps to the Mail tab (sd-771962)

- [x] 1.1 The unread-mail line carries a kind with the agent as its value, so it joins the focusable
      set, and says `⏎ read them` like the other actionable lines.
- [x] 1.2 ⏎ and `g` both route through `gotoItem`, so the two keys cannot come to mean different
      things about the same item.
- [x] 1.3 `showUnreadFor` sets both axes — recipient and unread — so the destination is the set the
      count counted.
- [x] 1.4 A jump from a foreign agent widens the repo scope and says so; a local one leaves the
      scope untouched, since it is TUI-wide and moving it is a side effect.
- [x] 1.5 Pinned: the item is reachable and absent when there is nothing unread, the destination row
      count matches the number, the foreign jump lands on its mail, and ⏎ jumps rather than opening
      the details modal.

## 2. An active filter says so, and one key clears it (sd-0e30bb)

- [x] 2.1 `filterLine` states every axis in force, and only when one of them is non-default.
- [x] 2.2 It is the third user of the unselectable-line mechanism sd-9c1436 settled — a `headingRow`
      through the one `listing` assembler, not a parallel path — so the cursor walks past it.
- [x] 2.3 Every list tab, each with the axes it actually has: Tasks its filter, Agents the scope,
      PRs and Runs both, Mail also the recipient. Tasks' scope is left out, being fixed to the repo
      whatever the toggle says.
- [x] 2.4 `esc` clears every axis to the tab's DEFAULT, not to the widest, and does nothing at all
      on a view that is not narrowed.
- [x] 2.5 An emptied list keeps its line, which is the case where "there is nothing" and "you
      filtered it all out" look identical without one.
- [x] 2.6 Pinned: silence on every tab's ordinary view, every axis named when shown, the line
      unselectable and first, esc clearing all of it, and esc still cancelling the space menu.
