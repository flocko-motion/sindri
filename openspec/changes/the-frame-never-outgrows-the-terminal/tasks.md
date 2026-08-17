# Tasks

## 1. Guard the contract first

- [x] 1.1 A test renders every tab at three terminal sizes, with the detail column shown and hidden,
      and asserts the frame is exactly the terminal's height and no line wider than its width. It
      fails before the fix, on Runs, by the height of its note.

## 2. Fix the slot

- [x] 2.1 `reclamp` chooses the detail pane's height where it already chooses the list's: the body
      height, or `runsPaneHeight()` on Runs.
- [x] 2.2 The unreachable `else if m.tab == 5` arm goes rather than being made reachable — it sized
      the viewport to the unwrapped line count, which fixes the height and breaks the scroll.

## 3. Pin the scroll too

- [x] 3.1 A test gives a run an output whose lines wrap, and asserts the viewport counts the wrapped
      lines the pane actually renders. It passes today, fails under the literal fix, and passes
      after this one — which is the whole argument for not resurrecting the dead arm.
