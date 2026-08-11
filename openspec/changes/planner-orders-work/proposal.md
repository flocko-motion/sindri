# A planner can order the work it plans

## Why

A planner could not express sequence. It proposes tasks; priority and approval were both the user's,
so it handed over a set of work with no way to say which comes first, and no way to re-order the
backlog it had planned.

The two were conflated because both gate claiming. They are different decisions: PRIORITY is intent
and ordering, APPROVAL is authorisation. Only the second releases work.

## The invariant

No planner action changes whether a task is claimable. Claimable is a priority AND authorisation, so
this is one boundary rather than five cases: a planner may move a priority between values, and may
rate anything the gate still holds; it may not cross between unrated and rated on a task the gate
would let out.

Both directions are refused for the same reason. Setting a first priority on an approved task
releases it to a worker; clearing one withdraws work already released. Approval stays entirely the
user's, directly and as a side effect.

## What changes

- `create-task --priority` proposes an order at birth.
- `prioritise-task <id> <level>` changes one afterwards.
- The refusal names which of the two crossings it would be.
- No worker nudge on a planner's rating. `SetPriority` wakes idle workers when a rating makes a task
  claimable; by the invariant a planner's never does, so a nudge there would always be waking
  someone for work they cannot take.
- The human approve surface names the priority the task carries, so the sequence being authorised is
  visible at the moment it starts handing work out.

## Decisions

**A separate verb, not `edit-task --priority`.** `edit-task` is pending-only because approval means
the user took the task AS IT STANDS — that rule is about content. Ordering is not content, and
re-sequencing live backlog is the job, so folding it in would have made `edit-task`'s rule depend on
which flags were passed and weakened it for the fields it protects.

**One task at a time, no scope.** The human's priority verb takes a scope (task / unrated / all). A
cascade from a planner would have to test the boundary per target, and one call could then flip many
tasks at once. The rule stays inspectable at the call site.

**The write order in `create-task` is part of the guarantee, and is the one thing here no test
pins.** A final-state assertion cannot see a transient window — reversing the order leaves an
identical row — so the mutation that reverses it passes. The code is correct by construction and
says why at the site; the gap is stated rather than papered over. A task carrying a rating with no
approval row IS claimable — the claim queries read `status IS NULL OR status='approved'`. Writing
the priority with the task would open a window, however short, in which a worker could take work the
user has never seen. The approval row is written first and the priority applied after.

**No word for unrated.** `none` is P4, the lowest rating, not the absence of one. The verb therefore
cannot clear a priority at all; the rule still forbids it, as defence for any other caller, and the
predicate is tested directly rather than through a path that cannot reach it.

## Consequences handled

- The `create-task` help said "approval and prioritisation are the two human decisions that release
  work". Reworded: approval is what releases work; a priority is a proposed ordering.
- `sd-5c1faf`'s confirm — offering to approve a task the user has just rated — is a HUMAN flow. I
  checked rather than assumed: both halves live in `internal/ui/tui` and `internal/ui/cli`, and a
  planner reaches the hub through registry verbs, so neither can fire for it. Nothing to change.

## Impact

- Specs: `hub` gains the requirement.
- Code: `internal/hub/workflow/plannerpriority.go` (the rule and the verb), `planner.go`
  (`--priority`, the write order, the help), `commands.go` (registration), and the two approve
  surfaces.
- `internal/api` gains the priority vocabulary — the words, their P-codes, and a strict parser —
  because the hub now parses a priority off an agent's command and must not import a `ui` package to
  do it. `internal/ui/theme` keeps the styling and delegates the table, so there is one list rather
  than a hub copy that could drift from what the menus offer.
