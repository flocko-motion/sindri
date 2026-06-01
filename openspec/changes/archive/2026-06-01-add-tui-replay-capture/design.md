## Context

The Bubble Tea TUI in `internal/tui` is headless-testable today — the
existing `internal/tui/list_view_test.go` already builds a `Model`, sets
state directly, and asserts on `m.viewList()`. The recent mutation-parity,
view-parity, and filter changes touched several states (merge confirm,
reject-reason input, status-filter cycle, the workers role column) that I
could not visually verify from this environment, and there is no
regression coverage for layout drift.

Two implementation models are available for a scripted driver:

- **In-process model driver.** Parse a script into `tea.Msg` values, apply
  via `Model.Update`, capture via `Model.View()`. Deterministic, fast, no
  TTY, no live data. The bugs we care about (key routing, state changes,
  layout math) live at Update/View; the runtime is charmbracelet's.
- **Real binary in a PTY/tmux.** Spawn `sindri tui`, send keystrokes,
  `tmux capture-pane -e`. True end-to-end, but needs real data, is
  timing-sensitive, and is flakier.

`charmbracelet/x/exp/teatest` exists as a middle ground (it runs the real
Program in a fake terminal with `Send` / `WaitFor` / golden output) but
its model is "send keys then assert final output" — it fits "capture at
arbitrary points" less cleanly than a small bespoke pump.

## Goals / Non-Goals

**Goals:**

- A deterministic, in-process driver for the existing `Model`, with a
  small key-sequence DSL and capture directives.
- Frame captures in two variants (raw ANSI + plain text).
- Golden-frame tests covering the freshly-touched TUI states so layout
  drift surfaces in `go test`.

**Non-Goals:**

- Real-PTY/tmux integration. The engine is in-process only. Real-binary
  capture remains a manual sanity check.
- Live `td` / `board.List` data. Fixtures only.
- Exhaustive UI coverage from day one. Start with the states we just
  touched and extend as needed.

## Decisions

- **In-process model driver over PTY/tmux:** Determinism, speed, no TTY
  or data dependency. The bugs we care about live at the Update/View
  layer this captures; the tea.Program runtime is upstream and not the
  source of our recent regressions.
- **Hand-rolled driver over teatest:** `teatest`'s "send keys then assert
  final output" shape fits less well than a small pump that supports
  `(capture <name>)` at arbitrary points. No new external dependency.
- **Fixture data, not live:** Reproducibility. Captures must be byte-equal
  across machines and across runs without a `td` binary or a `.git/pr/`
  store. A `--live` mode can be added later if it proves useful.
- **Both raw-ANSI and stripped captures:** Raw lets a human `cat` the
  file in a real terminal to see colours; stripped is what golden diffs
  compare against, where colour escape churn would be noise.
- **Drain pending Cmds rather than sleep:** Real time is a flake source.
  After each `Update`, run every returned `tea.Cmd` and feed its
  resulting `tea.Msg` back into `Update` until quiescence. `(sleep N)`
  is accepted as an alias for `(drain)` to keep the DSL readable.
- **DSL kept tiny:** literal runes, named keys, three directives —
  `(resize WxH)`, `(drain)`, `(capture <name>)`. Adding more is easy
  later; staying small keeps the parser trivial.
- **Force truecolor at engine start:** Set
  `lipgloss.SetColorProfile(termenv.TrueColor)` so captures contain the
  ANSI escapes a real terminal would receive, regardless of the host's
  `TERM` or `NO_COLOR`.
- **Where it lives:** engine in `internal/tui/replay/`, golden tests in
  `internal/tui`, fixtures in `internal/tui/replay/fixtures.go`. The
  optional `--script` flag is wired in `cmd/sindri/tui.go`. The engine
  is type `ui` (it sits inside the TUI package and uses its types).

## Risks / Trade-offs

- [Fidelity gap: the real `tea.Program` runtime is not exercised] →
  Mitigation: keep a documented manual `tmux capture-pane -e` recipe
  for occasional end-to-end checks. The golden suite stays on the
  in-process engine.
- [Golden churn on intentional layout edits] → Mitigation: captures are
  small, committed, and reviewed; an explicit regenerate path
  (`go test -update`) keeps the diff visible in code review.
- [Special-key table incomplete for keys the TUI binds] → Mitigation:
  cover the keys the existing TUI uses (the set is small and known) and
  fail loudly on unknown tokens so gaps are obvious.
- [Forcing truecolor changes the View output compared to a stripped
  terminal] → Mitigation: that is the point of the ANSI variant; the
  stripped `.txt` variant remains comparable across environments.

## Open Questions

- Expose `sindri tui --script` as a first-class flag now, or keep the
  engine test-only at first and add the flag only if ad-hoc runs prove
  valuable?
- Should fixtures be Go constructors in `fixtures.go` (compile-checked,
  easy to extend in code) or JSON in `testdata/fixtures/` (declarative,
  reviewable in isolation)? Lean Go for now.
- Color profile: always force truecolor in the engine, or honour
  `CLICOLOR_FORCE` / `NO_COLOR` for users who want plain captures?
