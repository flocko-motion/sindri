# Tasks

## 1. Offer the gate the rating did not open

- [x] 1.1 `approveAfterPriority` mirrors `priorityAfterApprove`, and fires only for a task the
      approval gate still holds.
- [x] 1.2 The rating write chains a follow-up through `taskOpDoneMsg`, the carrier the approve path
      already used, so the confirm opens against the board that write produced.
- [x] 1.3 The confirm says which act releases the work and what declining leaves behind.

## 2. Refuse to approve anything that was not asked for

- [x] 2.1 Never auto-approve: the offer is a confirm, and declining runs nothing.
- [x] 2.2 The confirm covers the one task, whatever the rating's scope. Pending children are
      counted and named; there is no subtree option to click through.

## 3. CLI parity

- [x] 3.1 `ratedApproval` mirrors `approvedPriority`: a rating on a pending task says it will not
      be handed out until approved, and names `task approve` rather than performing it.
- [x] 3.2 Pending children are reported with the `--subtasks` flag named, not applied.
- [x] 3.3 Silence when the task is approved, and when the backlog cannot be read — being wrong
      about what is owed is worse than saying nothing.

## 4. Pin it

- [x] 4.1 Offered when pending, nothing when approved or ungated, declining runs nothing, and the
      note carries the consequence.
- [x] 4.2 The bulk-approval guard from both sides: the note excludes the children, and the option
      list has no subtree entry.
- [x] 4.3 The CLI advice, its silence cases, and the unreadable backlog.
- [x] 4.4 Mutation-checked compilably: dropping the pending guard fails the approved-task test,
      and adding a subtree option fails the one-task test.
