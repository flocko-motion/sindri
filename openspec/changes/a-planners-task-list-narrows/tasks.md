# Tasks

## 1. Narrow the default, and say what it hid

- [x] 1.1 `task list` defaults to `active` — the same segment the TUI and Mail open on, and the one a
      planner plans against.
- [x] 1.2 `api.TaskListSummary` is the closing line: what was shown, and how many open and closed
      tasks exist in total. Shared with the host CLI, which had a version of it — one sentence, not two.
- [x] 1.3 A worker's summary counts what it may READ, not the fleet's backlog: quoting a total it is
      not being shown would describe someone else's listing.

## 2. Apply the filter

- [x] 2.1 `CmdTasks` calls `api.FilterTasks`, the predicate the host CLI has always used.
- [x] 2.2 Every segment stays reachable, so nothing the default hides is lost.

## 3. Refuse what the verb cannot do

- [x] 3.1 `parseTaskListFlags` accepts `--filter v` and `--filter=v`, as `create-task` does, and
      refuses anything else by name.
- [x] 3.2 A bad filter VALUE is refused too, through `api.ParseTaskFilter`, which names the set.
- [x] 3.3 A refusal prints the usage and NOT the listing — a listing alongside the complaint is what
      made every filter look identical.
- [x] 3.4 A flag where an id belongs is refused rather than answered as an unknown task.
- [x] 3.5 `TaskHelp` advertises the flag, so the verb list and the verb's own usage describe one
      surface.

## 4. Keep the tree honest

- [x] 4.1 `api.WithAncestors` gives a kept task its parents back, reporting which were added for that
      reason alone; the verb marks them `(context)` and accounts for them in the summary.

## 5. Pin it

- [x] 5.1 The bare listing IS the active one, and says which filter it used.
- [x] 5.2 Each filter admits a different set, and no two produce identical output — the symptom that
      read as a broken filter.
- [x] 5.3 Both spellings of the flag mean the same thing.
- [x] 5.4 An unknown flag, an unknown value, a valueless flag, a bare word and a short flag are each
      refused, name the accepted set, and print no rows.
- [x] 5.5 A flag in the id position is refused.
- [x] 5.6 A closed parent of an open subtask stays visible, marked, with the subtask still beneath it,
      and the summary accounts for the extra row.
- [x] 5.7 `api.MatchesFilter`'s leniency on an unknown VALUE is asserted intact, so the distinction
      cannot be tidied away in either direction.
- [x] 5.8 Bare `task` for a planner, coauthor and reviewer — the invocation TaskHelp advertises first,
      and the one the gate was green over. It panicked: an unbounded caller reached the slice that
      drops the `list` word with nothing to drop, since only a BOUNDED one is answered above it. The
      test asserts the bare listing is identical to `task list`, which also pins the claim that the
      default IS the active filter through the spelling a planner actually types.
