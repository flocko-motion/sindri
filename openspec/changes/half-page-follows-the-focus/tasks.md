# Tasks

## 1. One resolver, two speeds

- [x] 1.1 `ctrl+d`/`ctrl+u` half-page the focused column through `scrollTarget()`, the resolver
      `J`/`K` already use.
- [x] 1.2 With the list focused they keep moving the selection by half a body height.
- [x] 1.3 `halfPage` lives beside the resolver, so neither key case decides for itself.

## 2. Say what the keys do

- [x] 2.1 The keymap comment describes the focus rule instead of the tab special case it had.
- [x] 2.2 The comment at the key cases stops calling either column "the main pane" — the term reads
      as the list to a user and as the diff in the code, so it cannot carry a rule.

## 3. Pin it

- [x] 3.1 List focused: the selection moves and no pane scrolls under it.
- [x] 3.2 Detail focused: the pane scrolls, by more than a line key moves it, and the selection
      stays put.
- [x] 3.3 The PRs tab's metadata column half-pages when focused, leaving the diff where it was.
