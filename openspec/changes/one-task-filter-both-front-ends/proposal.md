# One task filter, offered by both front-ends

## Why

The TUI cycles four filters with `f`; `sindri task list` has none and always prints everything.
That is behaviour one interface has and the other cannot reach, which the interchangeable-interfaces
rule forbids — and it is what surfaced the gap: filter behaviour could not be diagnosed from the CLI
at all.

Worse than the missing flag is where the rule lives. The filter is entirely inside the TUI today:
`taskRows()` holds the switch, `recentlyChanged()` the recency test, `tui.go` the two-hour window.
That is domain logic in a front-end, and a CLI that grew its own copy would make "active" mean two
different things depending on where you looked.

## What changes

- The filter moves to `internal/api`, beside the pure functions already shared over the exchange
  types: the four values, the predicate, the window as one constant, and the order both front-ends
  present them in. The TUI switch and the new CLI flag become two callers of one definition.
- "Active" is written down rather than inferred: every task that is not done, PLUS every task whose
  status changed within the window. A union, which is what the TUI predicate always was.
- `sindri task list --filter <open|closed|all|active>`, defaulting to `all` — what the bare command
  has always printed, so no existing use changes. When a filter hides tasks the listing says so,
  since an empty result under `--filter closed` otherwise reads as an empty backlog.
- The TUI keeps its own default, `active`: a screen redrawn every few seconds is a view of what is
  happening, and a listing is a record of what exists.
- `isDone` and the done-word list in the TUI go: `api.DoneStatus` is the one place those three
  status words are named, and the row colouring reads it too.

## The hole this closes on the way

`recentlyChanged` returned false for a task with no timestamp, and the comment naming that case sat
in a front-end where no user would find it. openspec changes reach the board undated, so a recently
closed `os-` task never appeared under "active" — the filter that exists to show work just finished
was the one place it could not be seen.

The openspec source now dates each change from its files: the newest mtime under
`openspec/changes/<name>`, which is what ticking a box in `tasks.md` moves. GitHub issues already
carry their own `updatedAt`, so with this every mirrored source is dated. The remaining rule — an
undated task is never recent — is stated in the shared definition, where both front-ends read it.

## Impact

- Specs: `hub` gains a requirement for the shared filter (mirroring the arrangement one it already
  has); `view-tui`'s filter toggle is corrected — it still describes three states defaulting to
  open, while the code has cycled four defaulting to active for some time.
- Code: `internal/api/taskfilter.go` (new), `internal/api/task.go`, `internal/ui/tui/` (tui.go,
  tab_tasks.go, onkey.go, keys.go, theme.go), `internal/ui/cli/task.go`,
  `internal/adapter/tasks/spec/spec.go`.
- A change's mtimes are the checkout's on a fresh clone, so shortly after one every change reads as
  recently changed. That is stated where the dating happens, and it decays on its own.
