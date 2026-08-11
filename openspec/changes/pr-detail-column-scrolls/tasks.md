# Tasks

## 1. Diagnose before changing anything

- [x] 1.1 Prove the diff pane already scrolls — J/K, ctrl+d/ctrl+u, and across a poll. Two earlier
      readings blamed it; it was never the fault.
- [x] 1.2 Find the actual defect: the right column's viewport was a local variable, rebuilt at
      every render, so its offset could never leave zero.
- [x] 1.3 Rule out the cursor as a way to reach it — it stops only on actionable items, all of
      which sit above the fold, while the reviews and history are plain text.

## 2. Give the column a viewport

- [x] 2.1 `prMeta` on the model, sized in `reclamp` with Resize so a refresh preserves the offset.
- [x] 2.2 `prMetaLines` returns the wrapped lines and the highlight index together, since reclamp
      and the renderer must count the same rows.

## 3. Reach it with the keys that already exist

- [x] 3.1 `scrollTarget` picks the focused region on the PRs tab, the detail pane everywhere else.
- [x] 3.2 MODIFY the vi-navigation requirement so the absolute "never gated by focus" claim is
      narrowed in the SPEC, not just in this proposal. Both deltas archive into the same file, so
      prose here would have left the spec contradicting the product either way round.

## 4. Pin it

- [x] 4.1 The column scrolls when focused, the diff does not move with it, and the reverse.
- [x] 4.2 Both survive a poll.
- [x] 4.3 The diff pane's existing behaviour, so the fix cannot silently cost it.
- [x] 4.4 Mutation-checked: reverting scrollTarget to always-detail fails three of them.
