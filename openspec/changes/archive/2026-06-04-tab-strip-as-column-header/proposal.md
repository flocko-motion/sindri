# Tab strip moves into the column header

## Why

The list view had two redundant pieces of view-switching chrome:

1. The top-right of the title row showed `[T]asks  [W]orkers` with the
   active tab highlighted.
2. The content column's own header line showed `Tasks` or `Workers` in
   green — which named the *same* thing the strip above did, just
   without the hotkey hint.

User:

> the title *inside* the content shows the current Tab title in green
> (Tasks). I'd prefer if the title *inside* the content shows the
> current Tab title in green *with* the hotkey explained *and* in grey
> the nonactive titles *with* hotkey... so what's currently top right
> should be merged into the tab content title

## What Changes

- The content column's first line is now a **tab strip** that combines
  both pieces: `[T]asks  [W]orkers`, with the active tab rendered bold
  + highlight and the inactive tab dimmed. Both show the hotkey letter
  in brackets so the user learns the binding from the tab itself.
- The top-right of the title row no longer carries a separate
  `[T]asks  [W]orkers` selector — it's now just the project title.
- The old green `Tasks` / `Workers` header that was passed through
  `renderColumn`'s headerStyle is gone; the tab strip prepended to the
  column content takes its place.

## Impact

- Affected spec: `view-work-list` — the "Identical across interfaces"
  area gets an explicit "tab strip is the column header" requirement.
- Affected code: `internal/tui/backlog.go` — new `tabHeader` helper,
  `viewList` rebuilt around it; titleBar no longer contains the view
  selector.
- Goldens regenerated. The column's first row now reads
  `[T]asks  [W]orkers` (styling visible in ANSI, both names visible in
  the stripped .txt golden).
