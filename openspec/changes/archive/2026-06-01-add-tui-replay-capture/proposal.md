## Why

The Bubble Tea TUI can only be checked by a human at a real terminal today,
which leaves the planning agent (Claude) unable to verify layout fixes or
hotkey changes from its environment. The recent mutation-parity, view-parity,
filter, worker-column, and reject-reason changes all shipped without visual
verification — that gap is a real risk for the next round of TUI work.

## What Changes

- Add a scripted, in-process replay-and-capture engine that drives the TUI's
  `Model` headlessly from a small key-sequence DSL, with synthetic fixture
  data, and writes the rendered `View()` to files at chosen points.
- DSL tokens: literal runes (typing), named special keys (down/up/enter/esc/
  tab/space/ctrl+<letter>), and parenthesized directives — `(resize WxH)`,
  `(drain)`, `(capture <name>)`. `(sleep N)` is accepted as an alias for
  `(drain)` so the runtime stays deterministic.
- Capture frames in two variants: raw-ANSI (`<name>.ansi`, viewable with
  `cat` in a real terminal) and stripped plain-text (`<name>.txt`, suitable
  for diff-friendly golden tests).
- Wire golden-frame regression tests for the recently-touched states so
  `go test` catches layout drift: list view (default + each filter), item
  detail (task with PRs/gates/worker, plus spec-only), workers view (with
  role column), merge-confirm modal, reject-reason input.
- Optional: expose `sindri tui --script <file> --capture-dir <dir>` as a
  debug flag wired to the same engine, for ad-hoc layout/hotkey checks.

## Capabilities

### New Capabilities

- `tui-replay`: a scriptable, in-process replay of the TUI `Model` with
  synthetic fixture data and frame capture, used for layout/hotkey
  verification by humans and agents and as the substrate for golden-frame
  regression tests.

### Modified Capabilities

- (none — the existing view/action specs continue to describe the user-facing
  TUI; this change adds a verification capability, not new user behavior.)

## Impact

- New code: `internal/tui/replay/` (engine + DSL parser + key map + fixture
  helpers) and `internal/tui/testdata/frames/*` (committed golden frames).
- New test: golden-frame test in `internal/tui` driven by the engine.
- Optional: a `--script`/`--capture-dir` flag added to `sindri tui` in
  `cmd/sindri/tui.go`.
- No new third-party dependencies (lipgloss/termenv are already vendored).
- No effect on production behavior; this is a development/verification
  surface.
