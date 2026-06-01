# tui-replay Specification

## Purpose
TBD - created by archiving change add-tui-replay-capture. Update Purpose after archive.
## Requirements
### Requirement: Headless model driving

The replay engine SHALL drive the TUI's existing `Model` headlessly by
applying simulated `tea.Msg` values produced from a key-sequence DSL. It
SHALL NOT open a real terminal, spawn a subprocess, or rely on a TTY.

#### Scenario: No TTY needed

- **WHEN** a script is replayed
- **THEN** each token is converted to a `tea.Msg` and applied via
  `Model.Update`, and no real terminal or subprocess is opened

#### Scenario: Same model as production

- **WHEN** the replay drives a view
- **THEN** the rendered output comes from the same `Model.View()` code path
  the running TUI uses

### Requirement: Key-sequence DSL

The DSL SHALL support three token kinds: literal runes (each character is
one keypress), named special keys (at minimum: `down`, `up`, `left`,
`right`, `enter`, `esc`, `tab`, `space`, and `ctrl+<letter>`), and
parenthesized directives. Unknown tokens SHALL cause a parse error rather
than be silently ignored.

#### Scenario: Typing literal text

- **WHEN** the script contains `hello`
- **THEN** the engine dispatches five `tea.KeyMsg` events, one per rune

#### Scenario: Special key

- **WHEN** the script contains `ctrl+k`
- **THEN** a `tea.KeyMsg` representing Ctrl-K is dispatched

#### Scenario: Unknown token

- **WHEN** the script contains a token that is neither a known special key,
  a literal rune sequence, nor a recognised directive
- **THEN** parsing SHALL fail with an error naming the offending token

### Requirement: Frame capture directive

A `(capture <name>)` directive SHALL write the current `Model.View()` to
the engine's capture directory, producing two files: `<name>.ansi`
containing the raw ANSI output as it would appear in a real terminal, and
`<name>.txt` containing the same output with ANSI escapes stripped, for
diff-friendly comparison.

#### Scenario: Two-variant capture

- **WHEN** a script runs `(capture list)`
- **THEN** the engine writes `<dir>/list.ansi` and `<dir>/list.txt`, each
  containing the View output at that point

#### Scenario: Truecolor in the ANSI variant

- **WHEN** any capture is written
- **THEN** the ANSI variant contains the colour escapes the TUI produced,
  not a stripped or downgraded approximation

### Requirement: Resize directive

A `(resize W H)` directive SHALL inject a `tea.WindowSizeMsg` with the
given width and height, so a layout can be captured at chosen dimensions.

#### Scenario: Capture at fixed size

- **WHEN** a script runs `(resize 120 40)` followed by `(capture x)`
- **THEN** the captured frame reflects a 120x40 model size

### Requirement: Deterministic async handling

The engine SHALL drain returned `tea.Cmd` values by running them and
feeding their resulting messages back into `Update` until no pending
commands remain, rather than sleeping. The `(drain)` directive SHALL
trigger an explicit drain; `(sleep N)` SHALL be accepted as an alias for
`(drain)` so existing mental models work but no real time is consumed.

#### Scenario: Async refresh handled

- **WHEN** an action returns a `tea.Cmd` (for example, a refresh after a
  mutation)
- **THEN** the engine runs the command and applies the resulting message
  before the next script token is processed

#### Scenario: Sleep is a drain alias

- **WHEN** a script contains `(sleep 1)`
- **THEN** the engine drains pending commands and the wall-clock time
  spent is independent of the argument

### Requirement: Synthetic fixture data

Replays SHALL run against deterministic, in-memory fixture data
(`[]issue.Issue`, `[]worker.Worker`, and any PR records the view needs),
not against live `td`, `board.List`, or the on-disk PR store, so captures
are reproducible byte-for-byte across machines and runs.

#### Scenario: Reproducible captures

- **WHEN** the same script is replayed twice against the same fixture
- **THEN** both runs produce byte-identical capture files

#### Scenario: No environment dependency

- **WHEN** a replay runs in an environment with no `td` binary and no
  `.git/pr/` directory
- **THEN** the replay still succeeds because all data comes from the
  fixture

### Requirement: Golden-frame regression tests

The TUI test suite SHALL include golden frames covering each of the
currently-shipped TUI states: the work list under each filter (open,
all, closed), the item detail of a task with PRs / gates / worker, the
spec-only item detail, the workers view including the role column, the
merge-confirm modal, and the reject-reason input. `go test` SHALL fail
with a readable diff when any golden frame drifts.

#### Scenario: Layout regression detected

- **WHEN** a layout change alters the rendering of a covered state
- **THEN** `go test ./internal/tui/...` fails with a diff identifying the
  drifted golden

#### Scenario: Intentional update flow

- **WHEN** a golden frame intentionally changes
- **THEN** the test suite supports regenerating it (for example with
  `go test -update`) so the new content can be reviewed in the diff

