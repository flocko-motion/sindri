# A stalled reviewer is noticed and nudged

## Why

A reviewer that stops mid-review is invisible to the stall machinery, and two independent gaps kept it
that way. Both had to close; either alone is inert.

1. `Stalled()` could not return true for a reviewer. Its last line was
   `phase == "working" || (container != "" && ...)`, and a reviewer's phase is `"reviewing"` with no
   Task and no Container — `assignReview` writes the phase and nothing else. Measured against ori's
   real values: `phase="reviewing" container="" runtime="idle" still=1h` gave `false`, where the same
   dwell under `"working"` gave `true`.
2. `NudgeStalled` would still have bailed. It names the held work from `st.Task`, falling back to
   `st.Container`, and returns false when both are empty — which they always are for a reviewer,
   whose hold lives in the review row.

The sweep was blameless: it visits every agent, reviewers included. The rule rejected them.

## What changes

- `Stalled()` counts `"reviewing"`. `"submitted"` and `"gating"` are exempt because they EXIST to
  wait; reviewing does not — a reviewer with a PR is supposed to be reading it.
- `NudgeStalled` falls back to `ps.ReviewingPR(name)` when Task and Container are both empty, so
  `MsgStalled` has the PR to name and the log records which review went quiet. A store fault there
  sends nothing rather than nudging with a blank, and a `"reviewing"` phase with no live review row
  still produces silence rather than an invented id.
- The board says `stalled` for a stuck reviewer, because `stalledFor` deliberately feeds both the
  word the user reads and the prod the agent gets. That is how the condition becomes visible at all,
  and the same observation drives both, so the two can never disagree.

## Impact

- Specs: `05-workflow` gains a requirement for the reviewer half of the stall rule.
- Code: `internal/hub/workflow/stall.go` only. `hub/stallwatch.go` and the watchdog are untouched —
  they were already correct.
- A reviewer holding a PR while its screen stands still past the dwell is now prodded once per spell,
  named by its PR, and reads as `stalled` on the board.
- Nudging a reviewer that cannot act (a spend or session limit) is harmless and gets no special case:
  the push is one message, and the dwell is keyed per spell so it is not repeated.
