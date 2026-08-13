# Presentation pass: colours, labels and detail fields

## Why

Five small changes to how state is PRESENTED, landing on one branch because they touch the same
files — the row and detail rendering and the shared theme — and three of them share one rule.

That rule: colour by what the USER should do, where red means stopped and only the user can unstop
it. The agents palette already says exactly this; tasks and PRs are being brought into line with it.
And the invariant behind it: red and the `(N!)` attention badge are the same claim rendered twice,
so both must come from one predicate, or the colour drifts from the badge the first time a state is
added — invisibly, because each looks plausible alone.

## What changes

- **PR detail carries a lifecycle summary.** Twenty-two event types are logged against a PR and the
  detail rendered all of them, so the block that should say "where has this got to" was unreadable.
  The milestones — created, the verdicts, merged — now head the detail, and the full log stays below
  it, because the diagnostics it drops are what a failure needs. The classification lives in
  `internal/api` (both front-ends must agree on what a milestone is), verdict authorship comes from
  the review records rather than the payload prose, and an unclassified event type fails the build:
  a default either way is wrong in silence.

The remaining four subtasks (the "unapproved" label, PR row colours, task row colours, and the
last-changed timestamp in the task detail) land on this same branch and extend this document.

## Impact

- Specs: `hub` gains the classification and the projection; the colour rule follows with the
  subtasks that implement it.
- Code so far: `internal/api/prlifecycle.go` (new), `internal/ui/tui/tab_prs.go`,
  `internal/ui/cli/hub.go`, plus the fail-closed test in `internal/hub`.
