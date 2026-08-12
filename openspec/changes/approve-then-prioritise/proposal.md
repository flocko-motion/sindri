# Approving offers the priority, and a priority takes a scope

## Why

Two gates release work — the approval and the priority — and they are two separate
actions. Clearing the first FEELS like releasing the work, so a dozen approved tasks
sat in this backlog inert, each waiting on a second keypress nobody knew was owed.
Nothing on screen said so: an approved task with no priority reads as available and is
claimable by no one.

The priority also reaches exactly one task, and there is a case where that is the wrong
unit. `OpenLeaves` excludes a child only while its parent is open, so the moment a parent
closes over open children each child becomes a standalone leaf needing a rating of its
own. That state occurred here, and so did its sibling — a parent carrying no priority at
all — leaving approved children unclaimable with nothing explaining why.

A naive cascade would be worse than the gap, because it would MISLEAD. While the parent is
open its children are claimed as one package (`store/epicprio_test.go` pins this: rating
the epic alone releases it, and its children come along deliberately unrated). A child's
rating there only orders the subtasks `OpenSubtasks` hands out. An option that appeared to
release the children of a rated epic would be lying about what it had just done.

## What Changes

- After an approve, the priority is offered on the task just approved — the TUI chains its
  priority modal, the CLI takes `task approve <id> --priority P2`. The two existing actions
  are CHAINED, not merged: there is still one approve path and one priority path.
- The offer is made only where something is owed. A task already released by a rating of
  its own or an ancestor's is left alone, since asking about it is the friction in reverse.
- The priority takes a scope — this task only, every open task below it with no priority
  set, or every open task below it. The narrowest stays the default and today's behaviour.
- A cascade never touches a task that has ended: its rating decides nothing, and rewriting
  it would edit the record of work already done.
- Both front-ends state what carrying the rating down would ACTUALLY do for the task in
  hand: under an open parent it orders the subtasks; under a parent that has ended it is
  what makes each child claimable. One shared sentence, so neither front-end can invent a
  friendlier version of it.
- `td-8c0187` ("every open task must be claimable by someone") is the underlying trap these
  scopes work around, and is deliberately not solved here. Nothing added here stands in its
  way: the scopes are an explicit user action, and a rule that made a rated parent release
  its unrated children on closing would simply make the wider scopes unnecessary.

## Impact

- **Source of truth:** `internal/api/priority.go` (the scope value and `PriorityEffect`,
  the pure rule both front-ends state), `internal/hub/workflow/task.go` (`SetPriority`
  takes a scope), `internal/ui/theme/task.go` (the shared wording).
- **Wire:** `PriorityReq` gains `scope`; an unrecognised value is refused rather than
  narrowed, so a caller is never told a cascade happened when one did not.
- **Front-ends:** the TUI gains a scope step after the priority pick and a chained modal
  after approve; the CLI gains `task priority --scope` and `task approve --priority/--scope`.
- **Generic chrome:** the pick-one modal gains a wrapped `note` under its title, since a
  title sets the box's width and a sentence in one would make the box wider than the
  terminal. Every choice modal is now bounded by the screen.
- No change to what any claim query means. `OpenLeaves`, `OpenContainers` and
  `OpenSubtasks` are untouched — the scopes exist to feed them the ratings they already ask
  for.
