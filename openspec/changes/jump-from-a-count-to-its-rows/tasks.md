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

- [ ] (next subtask)
