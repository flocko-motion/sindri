# A planner's task list narrows, and refuses the flag it does not have

## Why

`task list` handed a planner the whole backlog: 177 rows in this repo, overwhelmingly closed history,
with the open work a handful of rows buried in the middle. That is the reading a planner does most.

Worse, the verb accepted any argument and ignored it. `--filter active`, `--filter bogus` and
`--wibble` all printed the same 177 rows and said nothing, so the filter looked broken when it was
simply absent — and cost a round of investigation in the wrong code path. Silence is the defect: a
caller that believes it narrowed the list and got the opposite has been told a lie by omission.

## What changes

- **The default is `active`** — open work plus whatever changed inside `ActiveWindow`. The same
  default the TUI and Mail already open on, and the set a planner plans against. `--filter all` is
  still the whole thing, one flag away.
- **The listing closes by saying what it showed and how big the backlog is**:
  `showing 3 open tasks — 3 open, 4 closed in total`. This is what makes a narrow default safe: a
  listing of fourteen rows can never read as a backlog of fourteen tasks. The wording is
  `api.TaskListSummary`, shared with the host CLI, which had a version of it
  (`(filter %s — %d of %d task(s) shown)`) — one sentence rather than two that drift.
- **The filter is applied at all.** `CmdTasks` now calls `api.FilterTasks`, the same pure predicate
  the host CLI has always used.
- **An unknown argument is refused**, naming what is accepted, and the listing does NOT print. A flag
  where an id belongs (`task --wibble`) is refused too, rather than read as a task nobody has heard
  of — "no such task" sends the reader looking for a task instead of at what they typed. Both spellings
  of the flag are accepted (`--filter v`, `--filter=v`), as `create-task` already does.
- **Ancestors of a shown task stay visible, marked `(context)`.** The listing is indented by tree, so
  an open subtask whose parent the filter dropped would be re-rooted and read as belonging to nobody —
  and which tree a task hangs in is most of what a planner reads this view for. The summary accounts
  for those rows, so the count above and the rows on screen agree.

## The leniency in `api.MatchesFilter` stays

It admits everything on a filter *value* it does not recognise, because "a listing that showed nothing
would read as an empty backlog, which is a lie a wrong flag should not be able to tell". That
reasoning is sound and untouched — there, showing too much is the safe failure. It does not extend to
an unknown *argument*: one is a lenient answer to a question that was understood, the other is not
answering the question at all. A test pins both halves so neither can be "tidied" into the other.

## The host CLI keeps its `all` default

`sindri task list` is deliberately different, and says why already: "a screen redrawn every few
seconds is a view, and a listing is a record." The agent's verb is a view — read over and over, mid
task — so it follows the TUI. Only the summary wording is now shared.

## Impact

- Specs: `agent-runtime` gains a requirement for what the agent's backlog reading shows and what it
  refuses, beside the thin-client requirement that governs the verb surface.
- Code: `internal/api/taskfilter.go` (the shared summary and the ancestor rule),
  `internal/hub/workflow/planner.go`, `internal/ui/cli/task.go`.
- `active` cannot be exercised through a hub fixture: `PutOwnedTask` stamps `updated_at` as it writes,
  so a seeded task cannot be back-dated and every fixture row reads as changed just now. The verb's
  tests therefore prove it applies a filter and refuses a bad one; what `active` admits is
  `api.MatchesFilter`'s rule, tested over a dated backlog in `internal/api`. The fixture says so, so
  the next reader does not take it for laziness.
