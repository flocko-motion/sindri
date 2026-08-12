# Rating a task the gate still holds releases nothing, and said so nowhere

## Why

All three claim queries require `(a.status IS NULL OR a.status = "approved")` alongside a
priority (`internal/hub/store/tasks.go`). So a task that is rated but still pending is exactly as
unclaimable as before the rating — while its row visibly gains the priority, which reads as work
released. The user performs one of the two gating actions, sees a change, and nothing is handed out.

`approve-then-prioritise` (sd-ee62df) closed the approve → priority direction, for the same reason
and after the same symptom: a dozen approved-but-unrated tasks sitting inert, each owing a second
keypress nobody knew about. This is its mirror, and the last of the pair.

## What changes

- Rating a task that is still pending offers the approval, in the same chained-modal shape the
  approve → priority direction already uses: `approveAfterPriority` mirrors `priorityAfterApprove`,
  and the rating write hands on through the carrier the approve path already had, so the follow-up
  runs against the board the write produced.
- The confirm states the consequence both ways: approving is what releases the task to a worker,
  the priority alone does not, and declining keeps the priority and leaves it in the backlog.
- An already-approved task is offered nothing. The prompt exists to explain a gate that is shut.
- Approval is never granted as a side effect. It decides what work exists, and one keystroke
  rating a task must not release it.
- A scoped rating approves nothing it reached. Children still pending are counted and named, and
  the confirm offers the one task only — no "task + subtasks" option, which would turn a rating
  into a bulk verdict on work the user ordered but never read.
- The CLI counterpart matches the shape sd-ee62df established there: `ratedApproval` mirrors
  `approvedPriority`, saying the task will not be handed out until approved, and naming
  `--subtasks` for pending children rather than applying it.

## Impact

- Specs: the `hub` requirement sd-ee62df extended for one direction now states both. This delta
  MODIFIES that requirement and carries sd-ee62df's own paragraph forward with it, so the two
  pending changes cannot leave the archived spec describing only half the pair.
- Code: `internal/ui/tui/approve_choice.go` (the mirror helper and confirm),
  `priority_choice.go` (the rating write chains a follow-up), `messages.go`, `tui.go`, and
  `internal/ui/cli/task.go`.
- `setPriorityCmd` now reports a `taskOpDoneMsg` rather than a bare refresh. That is the carrier
  the approve path already used to run a follow-up on fresh state, so this reuses one mechanism
  instead of adding a second.
- `ratedApproval` takes the one-method slice of the backend it needs and an `io.Writer`, so what
  it prints is testable without standing up a hub.
