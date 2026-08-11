# Tasks

## 1. The scope, and what it honestly does

- [x] 1.1 `api.PriorityScope` (`task`/`unrated`/`all`) with `ParsePriorityScope`,
      which refuses an unrecognised value rather than narrowing it, and reads ""
      as the narrowest so an old caller keeps today's behaviour.
- [x] 1.2 `api.PriorityEffect` — the counts a menu promises to change (open
      descendants at any depth, and how many carry no rating) plus `Independent`:
      whether the parent has ENDED, which is the whole reason the two cases are
      described differently.
- [x] 1.3 `theme.PriorityScopeNote` and `theme.PriorityScopeLabels` — the shared
      wording, so neither front-end can invent a friendlier version of it. Also
      `theme.Plural`, collapsing the copy the CLI and the TUI each kept rather than
      adding a third.

## 2. Applying it hub-side

- [x] 2.1 `workflow.Engine.SetPriority` takes a scope; `priorityTargets` resolves
      it CHILDREN FIRST (the parent's rating releases a package, so none can be
      claimed half-rated) over open descendants only.
- [x] 2.2 One nudge however far the cascade reached — waking an idle worker is all
      it does, and a message per task rated would be the same wake-up down a tree.
- [x] 2.3 `PriorityReq.Scope` on the wire; `POST /priority` refuses an unknown one.
- [x] 2.4 `client.SetPriority` carries the scope.

## 3. Approve hands on to the priority

- [x] 3.1 TUI: a successful approve chains the priority modal for the task just
      approved (`afterTaskOp` + `taskOpDoneMsg.then`), from both the direct key and
      the wider-approve modal — and only when nothing already rates the task
      (`api.ReleasedByPriority`).
- [x] 3.2 CLI: `task approve <id> --priority <p> [--scope …]` does both in one call;
      without it, an unrated task is named as claimable by nobody yet, with the flag
      and the standalone command both offered.

## 4. Parity

- [x] 4.1 TUI: the priority pick is followed by the scope step whenever open tasks
      sit below, carrying the honest note and the real counts in its labels.
- [x] 4.1a The pick-one modal gained a `note` — a sentence belongs under the title,
      wrapped, since a title IS the box's width and a sentence in one makes a box
      wider than the terminal. The box is now held inside the screen either way.
- [x] 4.2 CLI: `task priority <id> <p> --scope task|unrated|all`, printing how far it
      reached and the same note — including the pointer to the wider scopes when the
      rating stopped at the one task.

## 5. Spec

- [x] 5.1 `hub`: the approval requirement gains the second gate and its two
      scenarios.
- [x] 5.2 `hub`: a new requirement for the scope, what a cascade may not touch, and
      the two effects a front-end must state apart.

## 6. Verify

- [x] 6.1 `api`: PriorityEffect counts only open descendants; Independent tracks the
      parent having ended; ParsePriorityScope refuses the unknown.
- [x] 6.2 `workflow`: each scope's writes, closed tasks untouched, and the trap
      itself — a closed parent's children become claimable to `OpenLeaves` once
      rated, while a grandchild under a still-open child does not.
- [x] 6.3 `tui`: the scope step only over a tree; the note differs by case and never
      claims releasability; approve chains the picker only when one is owed, and a
      failed approve chains nothing.
- [x] 6.4 `make verify` and `openspec validate --all` pass.
