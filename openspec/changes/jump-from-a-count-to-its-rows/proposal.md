# Jump from a count to the rows behind it

## Why

A detail pane states a fact — "mail: 2 unread" — and offers no way to see what it is talking about.
The number is the beginning of a question the UI cannot answer: which two, from whom, saying what.

Make such a count a place you can go. Selecting it moves to the tab that holds the rows, with the
view already narrowed to exactly what the count counted.

Which requires the narrowing to be honest about itself: a filter the user did not set, arrived at by
navigation, is far more confusing than one they cycled to on purpose. So the second half is making
an active filter visible wherever a list is narrowed, and trivial to clear.

## What changes

- The agent detail's unread-mail line carries a kind, so the cursor reaches it, and ⏎ opens the Mail
  tab narrowed on BOTH axes at once — recipient and unread — which is the same question the count
  asked of the same set.
- A jump from a foreign agent widens the repo scope rather than landing on an empty list. The count
  belongs to the agent, and the recipient filter already pins the destination to that one agent, so
  dropping the scope lets nothing else in. The widening is said in the flash, since it changes a
  setting the user did not touch.
- A narrowed list says so in a line above its rows, naming every axis in force and the key that
  clears them — shown only when something is non-default, so the ordinary view is untouched. It goes
  in through the same `listing` assembler that adds the column labels, as the third user of that
  mechanism rather than a parallel path.
- `esc` clears every axis at once, back to the tab's defaults. Not to the widest view: "all" is
  itself a filter the user would then have to clear, so widening would leave the line on screen.

## Impact

- Specs: `view-tui` (a count leads to its rows).
- Code: `internal/ui/tui/tab_agents.go` (the item gains a kind), `internal/ui/tui/items.go`
  (`homeTab` and `gotoItem` route it), `internal/ui/tui/tab_mail.go` (`showUnreadFor` sets both
  axes and the scope), `internal/ui/tui/onkey.go` (⏎ jumps rather than opening the modal).
- The footer deliberately gains no `esc` row: it lists a binding whatever the state, so a permanent
  "clear filters" would advertise a key that usually does nothing. The line names it exactly when it
  works.
- Reuses the filters that already exist — `MatchesMailFilter` takes a recipient, and `w` already
  narrows by one. What was missing was setting them from somewhere else.
