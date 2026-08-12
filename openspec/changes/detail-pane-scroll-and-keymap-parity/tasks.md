# Tasks

## 1. Diagnose against the running UI, not the source comments

- [x] 1.1 Reproduce with the `Screenshot` harness: confirm `J`/`K` already
      scroll `m.detail` unconditionally (not gated by `rightFocus`), and that
      the reported symptom is best explained by missing discoverability, not a
      broken scroll.

## 2. Advertise the keys

- [x] 2.1 Add keymap rows for `J`/`K`, `g`/`G`, `y`/`Y`, `ctrl+d`/`ctrl+u`
      (global scope) and `h`/`l` (task-tab scope), grouped and terse to keep
      the footer budget in check.
- [x] 2.2 Fix `C-h/l`'s silent double meaning (it also registered bare `l`) by
      writing it `C-h/C-l`.
- [x] 2.3 Corrected in review: `ctrl+d`/`ctrl+u` do not scroll the detail pane
      on every tab — only PRs special-cases them that way (`onkey.go`'s
      `tab == 2` branch); elsewhere they half-page the LIST. Split into its
      own `{"C-d/C-u", "page"}` row rather than folding it into `J`/`K`'s
      "scroll detail" label, which would have been wrong on four of five tabs.

## 3. Make the guarantee real

- [x] 3.1 `TestEveryHandledLetterKeyIsInTheKeymap`: parse `onkey.go`'s two
      switch statements and `keys.go`'s constants via `go/ast`; every
      single-letter key `onKey` dispatches on must have a keymap row.
- [x] 3.2 Verify it actually catches a gap (temporarily remove a row, confirm
      the test fails, restore it).

## 4. Regression test for the pinned behaviour

- [x] 4.1 A task with a long comment thread; assert `J` reaches its last line
      from both the unfocused and the `ctrl+l`-focused starting state.

## 5. Spec

- [x] 5.1 `view-tui`: "vi navigation" names `J`/`K`; "Panes are fixed-height
      scrollable regions" states scrolling reaches content with no actionable
      item.

## 6. Verify

- [x] 6.1 `make verify` passes.
- [x] 6.2 `openspec validate --all` passes.
